package plugintrace

import (
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/apparentlymart/go-shquot/shquot"
)

// ClientLogTracer constructs a ClientTracer that will emit human-oriented log entries
// into the given logger when trace events occur.
//
// The format of these log entries is not customizable and may change in
// future versions. For more control, construct your own ClientTracer and
// build log messages yourself.
func ClientLogTracer(logger *log.Logger) *ClientTracer {
	return &ClientTracer{
		ProcessStart: func(cmd *exec.Cmd) {
			// We use POSIX shell quoting here just to get a nice readable
			// string representation of the args. We won't actually be running
			// this, so it doesn't matter that we'll be using POSIX-style
			// quoting on non-POSIX platforms.
			execStr := shquot.POSIXShell(cmd.Args)
			logger.Printf("launching plugin server %s", execStr)
		},

		ProcessRunning: func(proc *os.Process) {
			logger.Printf("plugin server process has pid %d", proc.Pid)
		},

		ProcessStartFailed: func(cmd *exec.Cmd, err error) {
			execStr, _ := shquot.POSIXShellSplit(cmd.Args)
			logger.Printf("failed to start plugin server %s: %s", execStr, err)
		},

		ProcessExited: func(state *os.ProcessState) {
			logger.Printf("plugin server process exited: %s", state)
		},

		TLSConfig: func(config *tls.Config, auto bool) {
			if auto {
				logger.Println("auto-negotiated TLS configuration")
			} else {
				logger.Println("TLS configuration from custom configuration function")
			}
		},

		ServerStarted: func(proc *os.Process, addr net.Addr, protoVersion int) {
			logger.Printf("server process (pid %d) is listening at %s address %s for protocol version %d", proc.Pid, addr.Network(), addr, protoVersion)
		},

		ServerStartTimeout: func(proc *os.Process, timeout time.Duration) {
			logger.Printf("timeout (%s) waiting for handshake from pid %d", timeout, proc.Pid)
		},

		Connect: func(addr net.Addr) {
			logger.Printf("connecting to plugin server at %s address %s", addr.Network(), addr)
		},

		Connected: func(addr net.Addr) {
			logger.Printf("connected to plugin server at %s address %s", addr.Network(), addr)
		},

		ConnectFailed: func(addr net.Addr, err error) {
			logger.Printf("failed to connect to %s address %s: %s", addr.Network(), addr, err)
		},

		Closing: func(proc *os.Process) {
			logger.Printf("closing plugin server with pid %d", proc.Pid)
		},
	}
}
