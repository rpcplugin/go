package rpcplugin

import (
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"os/exec"
	"time"

	"google.golang.org/grpc"
)

// ClientVersion is the interface to implement to launch a client for a
// particular protocol version. The client is the calling program that
// hosts one or more plugins.
type ClientVersion interface {
	// ClientProxy instantiates an instance of the plugin service's client
	// struct bound to the given connection, and returns that client proxy
	// object ready to use.
	//
	// There must be a single specific interface type that all returned client
	// proxies implement, so that the caller can know what to type-assert the
	// returned empty interface value to in order to get a useful client object.
	ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)
}

// ClientVersionFunc is a function type that implements interface ClientVersion.
type ClientVersionFunc func(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)

var _ ClientVersion = ClientVersionFunc(nil)

// ClientProxy implements interface ClientVersion.
func (fn ClientVersionFunc) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
	return fn(ctx, conn)
}

// ClientConfig is used to configure the behavior of a plugin client.
type ClientConfig struct {
	// Handshake configures the handshake settings that must agree with those
	// configured in the server.
	Handshake HandshakeConfig

	// ProtoVersions gives a Client implementation for each major protocol
	// version. The server will select the greatest version number that the
	// client and the server have in common, and then report its choice
	// as part of the handshake.
	ProtoVersions map[int]ClientVersion

	// Cmd is a not-yet-started exec.Cmd that is configured to launch a
	// specific plugin server executable. The given object must not be
	// used by the caller after it's been passed as part of a ClientConfig,
	// and will be modified in undefined ways by the rpcplugin package.
	Cmd *exec.Cmd

	// TLSConfig is used to set an explicit TLS configuration on the RPC client.
	// If this is nil, the client and server will negotiate temporary mutual
	// TLS automatically as part of their handshake.
	TLSConfig *tls.Config

	// StartTimeout is a time limit on how long the plugin is allowed to wait
	// before signalling that it is ready.
	//
	// If this is given as zero, it will default to one minute.
	StartTimeout time.Duration

	// Stderr, if non-nil, will recieve any data written by the child process
	// to its stderr stream.
	//
	// If Stderr is nil, any data written to the child's stderr is discarded.
	//
	// Stdout is not available because it's used exclusively by the plugin
	// handshake protocol.
	Stderr io.Writer
}

func (c *ClientConfig) setDefaults() {
	if c.StartTimeout == 0 {
		c.StartTimeout = 1 * time.Minute
	}

	if c.Stderr == nil {
		c.Stderr = ioutil.Discard
	}
}
