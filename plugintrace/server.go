package plugintrace

import (
	"context"
	"crypto/tls"
	"net"
)

// ServerTracer contains function pointers that, if set, will be called when
// certain events occur in a plugin server whose context has this object
// registered.
//
// Some trace functions recieve mutable data structures via pointers for
// efficiency. Making any modifications to those data structures is forbidden.
type ServerTracer struct {
	// TLSConfig is called when server TLS configuration is complete. If and
	// only if the auto-negotiation protocol was used to produce a single-use
	// certificate, auto is true.
	TLSConfig func(config *tls.Config, auto bool)

	// Listening is called once the server listener is configured, with the
	// address where it is listening and other negotiated parameters.
	Listening func(addr net.Addr, tlsConfig *tls.Config, protoVersion int)

	// InterruptIgnored is called if the server is monitoring interrupt
	// signals and such a signal is received. The count argument is how many
	// interrupts have been received since the server started.
	//
	// If the ServerConfig has NoSignalHandlers set, this function will never
	// be called.
	InterruptIgnored func(count int)

	// InvalidClientHandshakeVersion is called if the server finds an invalid
	// version number in the supported proto version list while performing
	// version negotiation. The given string is the invalid "number".
	InvalidClientHandshakeVersion func(invalid string)

	// VersionNegotationFailed is called if the server can find no proto
	// versions in common with the client using the negotiation protocol.
	// The argument is the set of version numbers the client supports.
	VersionNegotationFailed func(clientVersions []int)

	// GRPCServeError is called if the GRPC server exits with an error.
	GRPCServeError func(error)
}

type serverCtxKeyType int

const serverCtxKey serverCtxKeyType = 1

var noopServerTrace = &ServerTracer{}

// WithServerTracer creates a child of the given context that has the given
// ServerTracer attached to it.
//
// Callers must not modify any part of ServerTracer object after passing it
// to this function, or behavior is undefined.
func WithServerTracer(ctx context.Context, tracer *ServerTracer) context.Context {
	return context.WithValue(ctx, serverCtxKey, tracer)
}

// ContextServerTracer retrieves the ServerTracer object associated with the
// given context. If none is associated, a no-op tracer is returned.
//
// Do not modify any part of the returned tracer.
func ContextServerTracer(ctx context.Context) *ServerTracer {
	tracer, ok := ctx.Value(serverCtxKey).(*ServerTracer)
	if !ok {
		return noopServerTrace
	}
	return tracer
}
