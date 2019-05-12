package rpcplugin

import (
	"context"
	"crypto/tls"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/apparentlymart/go-ctxenv/ctxenv"
	"go.rpcplugin.org/rpcplugin/plugintrace"
	"google.golang.org/grpc"
)

// Serve starts up a plugin server and blocks while serving requests. It
// returns only if initialization failed or if the given context becomes
// "done".
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
	protoVersion, server := negotiateProtoVersion(ctx, config.ProtoVersions)
	if server == nil {
		return fmt.Errorf("plugin does not support any protocol versions supported by the host")
	}

	// TODO: Fill in the other two arguments once the rest of this function
	// is implemented.
	tracer.Listening(nil, nil, protoVersion)

	return nil
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
	ProtoVersions map[int]Server

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

// Server is the interface to implement to write a plugin server, which is the
// plugin that is launched by the client and recieves requests from it.
type Server interface {
	RegisterServer(*grpc.Server) error
}

// ServerFunc is a function type that implements interface Server
type ServerFunc func(*grpc.Server) error

var _ Server = ServerFunc(nil)

func (fn ServerFunc) RegisterServer(srv *grpc.Server) error {
	return fn(srv)
}

func negotiateProtoVersion(ctx context.Context, protoVersions map[int]Server) (version int, server Server) {
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
