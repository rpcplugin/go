package plugintrace

import (
	"crypto/tls"
	"log"
	"net"
	"strconv"
	"strings"
)

// ServerLogTracer constructs a ServerTracer that will emit human-oriented log entries
// into the given logger when trace events occur.
//
// The format of these log entries is not customizable and may change in
// future versions. For more control, construct your own ServerTracer and
// build log messages yourself.
func ServerLogTracer(logger *log.Logger) *ServerTracer {
	return &ServerTracer{
		TLSConfig: func(config *tls.Config, auto bool) {
			if auto {
				logger.Println("auto-negotiated TLS configuration")
			} else {
				logger.Println("TLS configuration from custom configuration function")
			}
		},

		Listening: func(addr net.Addr, tlsConfig *tls.Config, protoVersion int) {
			logger.Printf("protocol version %d listening on %s", protoVersion, addr)
		},

		InterruptIgnored: func(count int) {
			logger.Printf("ignored interrupt signal (attempt %d)", count)
		},

		InvalidClientHandshakeVersion: func(invalid string) {
			logger.Printf("invalid version string %q in client handshake", invalid)
		},

		VersionNegotationFailed: func(clientVersions []int) {
			if len(clientVersions) == 0 {
				logger.Println("version negotiation failed: client supports no protocol versions")
				return
			}
			vStrs := make([]string, len(clientVersions))
			for i, v := range clientVersions {
				vStrs[i] = strconv.Itoa(v)
			}
			logger.Printf("version negotiation failed: client supports only %s", strings.Join(vStrs, ", "))
		},

		GRPCServeError: func(err error) {
			logger.Printf("failed to start GRPC server: %s", err)
		},
	}
}
