package plugintrace

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"os/exec"
	"time"
)

// ClientTracer contains function pointers that, if set, will be called when
// certain events occur in a plugin client whose context has this object
// registered.
//
// Some trace functions recieve mutable data structures via pointers for
// efficiency. Making any modifications to those data structures is forbidden,
// and these pointers must be discarded before each function returns.
type ClientTracer struct {
	// ProcessStart is called just before the client launches the child process
	// where the plugin server will run. The argument is the command definition
	// it will use.
	ProcessStart func(cmd *exec.Cmd)

	// ProcessRunning is called after after the server process is started.
	ProcessRunning func(proc *os.Process)

	// ProcessStartFailed is called if the server process failed to start,
	// giving the error value describing the failure.
	ProcessStartFailed func(cmd *exec.Cmd, err error)

	// ProcessExited is called when a server process terminates.
	ProcessExited func(state *os.ProcessState)

	// TLSConfig is called when client TLS configuration is complete. If and
	// only if the auto-negotiation protocol was used to produce a single-use
	// certificate, auto is true.
	TLSConfig func(config *tls.Config, auto bool)

	// ServerStarted is called once the server process has successfully
	// completed the handshake protocol and is ready to be used.
	ServerStarted func(proc *os.Process, addr net.Addr, protoVersion int)

	// ServerStartTimeout is called if the server program doesn't complete
	// the handshake before the configured timeout.
	ServerStartTimeout func(proc *os.Process, timeout time.Duration)

	// Connect is called just before the client opens a connection to the
	// server's listen socket.
	Connect func(addr net.Addr)

	// Connected is called once a connection to the server's listen socket
	// is successfully established.
	Connected func(addr net.Addr)

	// ConnectFailed is called if connecting to the server's listen socket
	// returned an error.
	ConnectFailed func(addr net.Addr, err error)

	// Closing is called when a plugin instance is asked to shut down, before
	// the child process is killed.
	Closing func(proc *os.Process)
}

type clientCtxKeyType int

const clientCtxKey clientCtxKeyType = 1

var noopClientTrace = &ClientTracer{}

// WithClientTracer creates a child of the given context that has the given
// ClientTracer attached to it.
//
// Callers must not modify any part of ClientTracer object after passing it
// to this function, or behavior is undefined.
func WithClientTracer(ctx context.Context, tracer *ClientTracer) context.Context {
	return context.WithValue(ctx, clientCtxKey, tracer)
}

// ContextClientTracer retrieves the ClientTracer object associated with the
// given context. If none is associated, a no-op tracer is returned.
//
// Do not modify any part of the returned tracer.
func ContextClientTracer(ctx context.Context) *ClientTracer {
	tracer, ok := ctx.Value(clientCtxKey).(*ClientTracer)
	if !ok {
		return noopClientTrace
	}
	return tracer
}
