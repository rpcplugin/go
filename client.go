package rpcplugin

import (
	"context"
	"crypto/tls"
	"os/exec"

	"google.golang.org/grpc"
)

// Client is the interface to implement to write a plugin client, which is the
// host program that launches plugins and initiates requests to them.
type Client interface {
	// ClientProxy instantiates an instance of the plugin service's client
	// struct bound to the given connection, and returns that client proxy
	// object ready to use.
	//
	// The caller must know which concrete type it will receive and type-assert
	// the return value to obtain a usable client proxy object.
	ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)
}

// ClientFunc is a function type that implements interface Client
type ClientFunc func(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)

var _ Client = ClientFunc(nil)

// ClientProxy implements interface Client.
func (fn ClientFunc) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
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
	ProtoVersions map[int]Client

	// Cmd is a not-yet-started exec.Cmd that is configured to launch a
	// specific plugin server executable. The given object must not be
	// used by the caller after it's been passed as part of a ClientConfig,
	// and will be modified in undefined ways by the rpcplugin package.
	Cmd *exec.Cmd

	// TLSConfig is used to set an explicit TLS configuration on the RPC client.
	// If this is nil, the client and server will negotiate temporary mutual
	// TLS automatically as part of their handshake.
	TLSConfig *tls.Config
}
