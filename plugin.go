package rpcplugin

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/apparentlymart/go-ctxenv/ctxenv"
	"go.rpcplugin.org/rpcplugin/plugintrace"
	"google.golang.org/grpc"
	grpcCreds "google.golang.org/grpc/credentials"
)

// Plugin represents a currently-active plugin instance, with an associated
// child process that is running an RPC server.
type Plugin struct {
	protoVersion int
	cv           ClientVersion
	process      *os.Process
	addr         net.Addr
	tlsConfig    *tls.Config
	exit         <-chan struct{}
	tracer       *plugintrace.ClientTracer
}

// New launches a plugin server in a child process and returns an object
// representing that ret.
//
// Once a ClientConfig has been passed to this function, the caller must no
// longer access it or modify it.
//
// If this function returns without error, the caller must retain the plugin
// object in order to eventually call Close on it, which will shut down the
// child process.
//
// The child process inherits the environment variables of the current process.
// To customize the child process environment for testing, use package
// package github.com/apparentlymart/go-envctx/envctx to set a different
// environment on the given context.
func New(ctx context.Context, config *ClientConfig) (plugin *Plugin, err error) {
	config.setDefaults()

	if len(config.ProtoVersions) == 0 {
		return nil, fmt.Errorf("config field ProtoVersions must have at least one version")
	}
	if config.Handshake.CookieKey == "" {
		return nil, fmt.Errorf("config field Handshake.CookieKey must not be empty")
	}
	if config.Handshake.CookieValue == "" {
		return nil, fmt.Errorf("config field Handshake.CookieValue must not be empty")
	}
	if config.Cmd == nil {
		return nil, fmt.Errorf("config field Cmd must not be nil")
	}

	var versionStrings []string
	for v := range config.ProtoVersions {
		versionStrings = append(versionStrings, strconv.Itoa(v))
	}

	environ := []string{
		fmt.Sprintf("%s=%s", config.Handshake.CookieKey, config.Handshake.CookieValue),
		fmt.Sprintf("PLUGIN_MIN_PORT=%d", config.MinPort),
		fmt.Sprintf("PLUGIN_MAX_PORT=%d", config.MaxPort),
		fmt.Sprintf("PLUGIN_PROTOCOL_VERSIONS=%s", strings.Join(versionStrings, ",")),
	}

	tlsConfig := config.TLSConfig
	autoTLS := false
	if tlsConfig == nil {
		// A nil TLSConfig means to use the auto-negotiation protocol.
		cert, err := generateCertificate(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate client TLS certificate: %s", err)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ServerName:   "localhost",
		}
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Certificate[0],
		})
		environ = append(environ, fmt.Sprintf("PLUGIN_CLIENT_CERT=%s", certPEM))
		autoTLS = true
	}

	config.Cmd.Env = append(environ, ctxenv.Environ(ctx)...)
	config.Cmd.Stdin = bytes.NewReader(nil)
	config.Cmd.Stderr = config.Stderr
	cmdStdout, err := config.Cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cannot create stdout pipe: %s", err)
	}

	tracer := plugintrace.ContextClientTracer(ctx)

	if tracer.ProcessStart != nil {
		tracer.ProcessStart(config.Cmd)
	}
	err = config.Cmd.Start()
	if err != nil {
		if tracer.ProcessStartFailed != nil {
			tracer.ProcessStartFailed(config.Cmd, err)
		}
		return nil, fmt.Errorf("failed to start child process: %s", err)
	}
	if tracer.ProcessRunning != nil {
		tracer.ProcessRunning(config.Cmd.Process)
	}

	exitCh := make(chan struct{})
	ret := &Plugin{
		process:   config.Cmd.Process,
		exit:      exitCh,
		tracer:    tracer,
		tlsConfig: tlsConfig,
	}

	go func(exit chan<- struct{}) {
		state, _ := ret.process.Wait()
		if state != nil && tracer.ProcessExited != nil {
			tracer.ProcessExited(state)
		}
		close(exit)
	}(exitCh)

	defer func() {
		p := recover()

		if err != nil || p != nil {
			ret.process.Kill()
		}

		if p != nil {
			panic(p)
		}
	}()

	// We'll use a goroutine to read stdout lines so that we can also watch
	// for our timeout to elapse.
	stdoutCh := make(chan string)
	go func(stdout io.ReadCloser) {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			stdoutCh <- sc.Text()
		}
		close(stdoutCh)
		stdout.Close()
	}(cmdStdout)

	timeout := time.After(config.StartTimeout)
	select {
	case <-timeout:
		if tracer.ServerStartTimeout != nil {
			tracer.ServerStartTimeout(ret.process, config.StartTimeout)
		}
		return nil, fmt.Errorf("timeout waiting for plugin server handshake message")
	case <-exitCh:
		return nil, fmt.Errorf("plugin server process exited without completing handshake")
	case line := <-stdoutCh:
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 5 {
			return nil, fmt.Errorf("invalid handshake message %q from plugin server", line)
		}

		// Verify the rpcplugin handshake version
		if parts[0] != "1" {
			return nil, fmt.Errorf("invalid handshake version %q from plugin server; want \"1\"", parts[0])
		}

		// Verify the RPC protocol selection
		if parts[4] != "grpc" {
			return nil, fmt.Errorf("invalid RPC protocol %q from plugin server; want \"grpc\"", parts[0])
		}

		// Verify the selected protocol version
		{
			version, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid protocol version %q from plugin server", parts[1])
			}

			cv, ok := config.ProtoVersions[version]
			if !ok {
				return nil, fmt.Errorf("plugin server selected unsupported protocol version %d", version)
			}
			ret.protoVersion = version
			ret.cv = cv
		}

		// Verify transport protocol and address
		switch parts[2] {
		case "tcp":
			addr, err := net.ResolveTCPAddr("tcp", parts[3])
			if err != nil {
				return nil, fmt.Errorf("plugin server provided invalid TCP socket address %q", parts[3])
			}
			ret.addr = addr
		case "unix":
			addr, err := net.ResolveUnixAddr("unix", parts[3])
			if err != nil {
				return nil, fmt.Errorf("plugin server provided invalid Unix socket address %q", parts[3])
			}
			ret.addr = addr
		default:
			return nil, fmt.Errorf("plugin server selected unsupported transport protocol %q", parts[2])
		}

		// parts[5] is the optional auto-generated server TLS certificate.
		// It must be at least 50 characters long to distinguish it from
		// other uses of this field in older hashicorp/go-plugin versions,
		// though rpcplugin's server does not ever produce such things.
		if len(parts) >= 6 && len(parts[5]) > 50 {
			certStr := parts[5]

			certPool := x509.NewCertPool()
			asn1, err := base64.RawStdEncoding.DecodeString(certStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse plugin server's temporary certificate: %s", err)
			}

			x509Cert, err := x509.ParseCertificate([]byte(asn1))
			if err != nil {
				return nil, fmt.Errorf("failed to parse plugin server's temporary certificate: %s", err)
			}

			certPool.AddCert(x509Cert)

			// The client will accept only this temporary certificate.
			ret.tlsConfig.RootCAs = certPool
		}

		if tracer.TLSConfig != nil {
			tracer.TLSConfig(ret.tlsConfig, autoTLS)
		}

		if tracer.ServerStarted != nil {
			tracer.ServerStarted(ret.process, ret.addr, ret.protoVersion)
		}

		return ret, nil
	}
}

// Client returns a client object that can be used to call plugin functions.
//
// The protoVersion return value is the protocol version negotiated with the
// plugin server. The client return value must be type-asserted by the caller
// to the appropriate GRPC client interface type for the negotiated protocol
// version.
func (p *Plugin) Client(ctx context.Context) (protoVersion int, client interface{}, err error) {
	tracer := p.tracer

	if tracer.Connect != nil {
		tracer.Connect(p.addr)
	}

	conn, err := grpc.DialContext(
		ctx, "", // address string is unused because we access p.addr for that
		grpc.FailOnNonTempDialError(true),
		grpc.WithTransportCredentials(grpcCreds.NewTLS(p.tlsConfig)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(math.MaxInt32)),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			addr := p.addr
			return net.Dial(addr.Network(), addr.String())
		}),
	)
	if err != nil {
		if tracer.ConnectFailed != nil {
			tracer.ConnectFailed(p.addr, err)
		}
		return 0, nil, fmt.Errorf("failed to connect to %s: %s", p.addr, err)
	}

	client, err = p.cv.ClientProxy(ctx, conn)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create client proxy: %s", err)
	}

	if tracer.Connected != nil {
		tracer.Connected(p.addr)
	}

	return p.protoVersion, client, nil
}

// Close terminates the plugin child process.
//
// After this function returns, the recieving plugin object is no longer valid
// and calling any methods on it will lead to undefined behavior, possibly
// including panics.
func (p *Plugin) Close() error {
	tracer := p.tracer

	if tracer.Closing != nil {
		tracer.Closing(p.process)
	}

	err := p.process.Kill()
	if err != nil {
		return fmt.Errorf("failed to kill pid %d: %s", p.process.Pid, err)
	}

	// Wait for the process to actually exit
	<-p.exit

	return nil
}
