package rpcplugin

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/apparentlymart/go-ctxenv/ctxenv"
	"go.rpcplugin.org/rpcplugin/plugintrace"
	"google.golang.org/grpc"
)

// Serve starts up a plugin server and blocks while serving requests. It
// returns an error if initialization failed or if the given context becomes
// "done". It returns nil once all in-flight requests are complete if the
// client asks the server to exit.
//
// Usually an application with rpcplugin-based plugins will have its own
// SDK library that provides a higher-level Serve function which takes some
// application-specific arguments. That allows the SDK to handle
// application-specific decisions about how the plugin RPC channel is configured,
// so that individual plugin implementers don't need to worry about it.
//
// Serve assumes ownership of the standard I/O handles and consumes several
// environment variables in order to implement the client/server negotiation
// protocol. If you need to override its use of the real process environment
// for testing purposes, use package github.com/apparentlymart/go-envctx/envctx
// to override the environment variable values via the context.
//
// Serve will also, by default, install a handler for the Interrupt signal
// (SIGINT on Unix-style systems) and ignore it, under the expectation that
// the plugin client process will handle interruptions and signal the
// plugins to shutdown via a different channel. This behavior can be overridden
// in ServerOpts if you need different signal-handling behavior.
func Serve(ctx context.Context, config *ServerConfig) error {
	if config.Handshake.CookieKey == "" || config.Handshake.CookieValue == "" {
		return fmt.Errorf("ServerConfig.Handshake must have non-empty CookieKey and CookieValue")
	}
	if !haveHandshakeCookie(ctx, &config.Handshake) {
		return NotChildProcessError
	}

	tracer := plugintrace.ContextServerTracer(ctx)
	protoVersion, server := negotiateServerProtoVersion(ctx, config.ProtoVersions)
	if server == nil {
		return fmt.Errorf("plugin does not support any protocol versions supported by the host")
	}

	listener, err := serverListen(ctx)
	if err != nil {
		return fmt.Errorf("cannot start plugin RPC server: %s", err)
	}
	defer listener.Close()

	var autoCertStr string // only populated if we use automatic certificate negotiation
	tlsConfig, autoCert, err := serverTLSConfig(ctx, listener.Addr(), config.TLSConfig)
	if err != nil {
		return fmt.Errorf("invalid TLS settings: %w", err)
	}
	if len(autoCert.Certificate) != 0 {
		if clientSmellsLikeGoPlugin(ctx) {
			// As a concession to go-plugin compatibility we use its non-standard
			// unpadded base64 encoding when the client seems like it's go-plugin,
			// or else the certificate won't be parsed correctly when its length
			// isn't a round 3 bytes.
			autoCertStr = base64.RawStdEncoding.EncodeToString(autoCert.Certificate[0])
		} else {
			autoCertStr = base64.StdEncoding.EncodeToString(autoCert.Certificate[0])
		}
	}
	if tracer.TLSConfig != nil {
		tracer.TLSConfig(tlsConfig, autoCertStr != "")
	}

	// While the plugin code is running we redirect os.Stdout and os.Stderr to
	// some pipes whose data we'll send via the RPC protocol, so that the "real"
	// stdout and stderr can be reserved for the plugin handshake data.
	handshakeOut := os.Stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %s", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %s", err)
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	chiCtx, cancel := context.WithCancel(ctx)
	srvGRC := &serverGRPC{
		Server: server,
		TLS:    tlsConfig,
		Stdout: stdoutR,
		Stderr: stderrR,
		Done:   cancel,
		Tracer: tracer,
	}
	var goPluginClose func()
	if clientSmellsLikeGoPlugin(ctx) {
		goPluginClose = cancel
	}
	err = srvGRC.Init(goPluginClose)
	if err != nil {
		return fmt.Errorf("plugin server init failed: %s", err)
	}

	// By default we eat SIGINT because otherwise we'll tend to get these
	// when the user tries to interrupt the host program, but we want to let
	// the host program be in control of when and how we shut down.
	if !config.NoSignalHandlers {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		go func() {
			var count int32
			ign := tracer.InterruptIgnored
			for {
				select {
				case <-ch:
					newCount := atomic.AddInt32(&count, 1)
					if ign != nil {
						ign(int(newCount))
					}
				case <-chiCtx.Done():
					return
				}
			}
		}()
	}

	// We must now write the rpcplugin handshake line to real stdout so that the
	// client (our parent process) knows where to connect.
	_, err = fmt.Fprintf(handshakeOut, "1|%d|%s|%s|grpc|%s\n",
		protoVersion,
		listener.Addr().Network(),
		listener.Addr().String(),
		autoCertStr,
	)
	if err != nil {
		return fmt.Errorf("failed to print plugin handshake to stdout: %s", err)
	}
	// We intentionally ignore the error from sync because stdout might be
	// bound to something that cannot sync.
	handshakeOut.Sync()

	go srvGRC.Serve(listener)

	if tracer.Listening != nil {
		tracer.Listening(listener.Addr(), tlsConfig, protoVersion)
	}
	<-chiCtx.Done() // wait for the GRPC handler to signal that it is ready to exit
	if chiCtx.Err() == context.Canceled {
		// For this particular context, being cancelled is not considered an error.
		return nil
	}
	return chiCtx.Err()
}

// ServerConfig is used to configure the behavior of a plugin server started
// by the Serve function.
type ServerConfig struct {
	// Handshake configures the handshake settings that must agree with those
	// configured in the client.
	Handshake HandshakeConfig

	// ProtoVersions gives a Server implementation for each major protocol
	// version. The server will select the greatest version number that the
	// client and the server have in common, and then call that version's
	// Server implementation to activate it.
	ProtoVersions map[int]ServerVersion

	// TLSConfig can be assigned a custom function for preparing the TLS
	// configuration used to authenticate and encrypt the RPC channel. If
	// no function is assigned, the ad-hoc TLS negotation protocol is used
	// to automatically establish a single-use key and certificate for each
	// plugin process.
	TLSConfig func() (*tls.Config, error)

	// Set NoSignalHandlers to prevent Serve from configuring the handling
	// of signals for the process. If you do this, you must find some other
	// way to prevent an interrupt signal to the client process group from also
	// being recieved by the plugin server processes.
	NoSignalHandlers bool
}

// ForceServerWithoutTLS is a predefined function for use with ServerConfig.TLSConfig
// which makes a server not use TLS at all. This makes the server non-compliant
// with the rpcplugin specification, but can be useful for debugging or for
// implementing servers for HashiCorp's go-plugin library when the client is
// not configured to use TLS. (That go-plugin mode isn't included in the
// rpcplugin specification.)
var ForceServerWithoutTLS = func() (*tls.Config, error) {
	return nil, errForceNoTLS
}

// errForceNoTLS is a special error type used by ServerWithoutTLS, so that
// ServerWithoutTLS will be the only possible way to turn of TLS mode and thus
// make the server non-rpcplugin-compliant,
var errForceNoTLS = errors.New("force no TLS")

// ServerVersion is the interface to implement to write a server for a particular
// plugin version.
type ServerVersion interface {
	RegisterServer(*grpc.Server) error
}

// ServerVersionFunc is a function type that implements interface ServerVersion
type ServerVersionFunc func(*grpc.Server) error

var _ ServerVersion = ServerVersionFunc(nil)

// RegisterServer implements ServerVersion.
func (fn ServerVersionFunc) RegisterServer(srv *grpc.Server) error {
	return fn(srv)
}

func negotiateServerProtoVersion(ctx context.Context, protoVersions map[int]ServerVersion) (version int, server ServerVersion) {
	clientVersionsStr := ctxenv.Getenv(ctx, "PLUGIN_PROTOCOL_VERSIONS")
	if clientVersionsStr == "" {
		// Client isn't performing the negotiation protocol propertly, so
		// negotiation fails.
		trace := plugintrace.ContextServerTracer(ctx)
		if trace.InvalidClientHandshakeVersion != nil {
			trace.InvalidClientHandshakeVersion("") // treat the empty string as a single empty version number
		}
		return 0, nil
	}

	vStrs := strings.Split(clientVersionsStr, ",")
	clientVersions := make([]int, 0, len(vStrs))
	for _, vStr := range vStrs {
		v, err := strconv.Atoi(vStr)
		if err != nil {
			trace := plugintrace.ContextServerTracer(ctx)
			if trace.InvalidClientHandshakeVersion != nil {
				trace.InvalidClientHandshakeVersion(vStr)
				continue
			}
		}
		clientVersions = append(clientVersions, v)
	}

	// We will try out the versions in reverse order so that we'll select
	// the newest one we have in common with the client.
	sort.Sort(sort.Reverse(sort.IntSlice(clientVersions)))

	for _, v := range clientVersions {
		if server, ok := protoVersions[v]; ok {
			return v, server
		}
	}

	// If we fall out here then we found nothing in common, so negotiation fails.
	trace := plugintrace.ContextServerTracer(ctx)
	if trace.VersionNegotationFailed != nil {
		trace.VersionNegotationFailed(clientVersions)
	}
	return 0, nil
}

// clientSmellsLikeGoPlugin returns true if the client hasn't set some of
// the environment variables that are required for rpcplugin but are not
// used by the (undocumented) HashiCorp go-plugin protocol.
//
// When this returns true, the server implementation will make some
// small not-rpcplugin-complient adaptations to make its behavior more
// compatible with go-plugin.
func clientSmellsLikeGoPlugin(ctx context.Context) bool {
	// go-plugin doesn't use the PLUGIN_TRANSPORTS environment variable and
	// instead expects servers to just "know" that the client expects
	// "unix,tcp" on Unix platforms and just "tcp" on Windows.
	return ctxenv.Getenv(ctx, "PLUGIN_TRANSPORTS") == ""
}
