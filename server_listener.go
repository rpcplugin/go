package rpcplugin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/apparentlymart/go-ctxenv/ctxenv"
	"go.rpcplugin.org/rpcplugin/plugintrace"
)

func serverTLSConfig(ctx context.Context, fn func() (*tls.Config, error)) (*tls.Config, tls.Certificate, error) {
	tracer := plugintrace.ContextServerTracer(ctx)
	if fn != nil {
		// If we're given a configuration function, it overrides all of our
		// usual default behavior so that the calling application can handle
		// TLS certificate selection/issuance however it wants.
		tlsConfig, err := fn()
		if err == nil && tlsConfig == nil {
			// Having no TLS config at all is not permitted.
			return nil, tls.Certificate{}, fmt.Errorf("TLS configuration function returned no TLS configuration")
		}
		if tracer.TLSConfig != nil {
			tracer.TLSConfig(tlsConfig, false)
		}
		return tlsConfig, tls.Certificate{}, err
	}

	// Automatic temporary certificate setup protocol
	clientCert := ctxenv.Getenv(ctx, "PLUGIN_CLIENT_CERT")
	if clientCert == "" {
		return nil, tls.Certificate{}, fmt.Errorf("PLUGIN_CLIENT_CERT environment variable is not set")
	}

	clientCertPool := x509.NewCertPool()
	if !clientCertPool.AppendCertsFromPEM([]byte(clientCert)) {
		return nil, tls.Certificate{}, fmt.Errorf("PLUGIN_CLIENT_CERT has invalid PEM certificate chain")
	}

	serverCert, err := generateCertificate(ctx)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("cannot create temporary server certificate: %s", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCertPool,
		MinVersion:   tls.VersionTLS12,
	}, serverCert, nil
}

func serverListen(ctx context.Context) (net.Listener, error) {
	// We prefer to use a UNIX domain socket if available because then we
	// can use filesystem permissions to further constraint access, but
	// not all operating systems support them so we'll fall back on a TCP
	// socket on loopback otherwise. Mutual TLS auth is the primary means
	// to keep other callers out of our server, so it doesn't really matter
	// if other processes on the same system can connect.
	l, err := serverListenUnix(ctx)
	if err == nil {
		return l, nil
	}

	return serverListenTCP(ctx)
}

func serverListenUnix(ctx context.Context) (net.Listener, error) {
	baseDir := ""
	if runtimeDir := ctxenv.Getenv(ctx, "XDG_RUNTIME_DIR"); runtimeDir != "" && filepath.IsAbs(runtimeDir) {
		// If XDG_RUNTIME_DIR is available then we'll prefer it, because its
		// permissions tend to be more suitable (per the contract for this
		// environment variable) and it'll get cleaned up on reboot if anything
		// goes wrong that prevents us from cleaning it ourselves.
		baseDir = runtimeDir
	}

	socketDir, err := ioutil.TempDir(baseDir, "rpcplugin")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory for plugin server socket: %s", err)
	}

	socketPath := filepath.Join(socketDir, "server.sock")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		os.RemoveAll(baseDir)
		return nil, fmt.Errorf("failed to open listener at %s: %s", socketPath, err)
	}

	// wrap for cleanup on close
	return &rmListener{
		Listener: l,
		Path:     socketDir,
	}, nil
}

func serverListenTCP(ctx context.Context) (net.Listener, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to open listener on 127.0.0.1: %s", err)
	}
	return l, nil
}

// rmListener is an implementation of net.Listener that forwards most
// calls to the listener but also removes a file or directory as part of
// closing. This allows us to clean up our temporary directory containing a
// UNIX socket.
type rmListener struct {
	net.Listener
	Path string
}

func (l *rmListener) Close() error {
	if err := l.Listener.Close(); err != nil {
		return err
	}

	return os.RemoveAll(l.Path)
}
