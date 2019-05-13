package rpcplugin

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"go.rpcplugin.org/rpcplugin/plugintrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// This is the name of the grpc service we use for our internal signalling,
// separate from the caller's RPC channel.
//
// Ideally we'd call this "rpcplugin", but we're inheriting this service name
// from HashiCorp's go-plugin to retain wire compatibilty.
const grpcServiceName = "plugin"

type serverGRPC struct {
	Server Server
	TLS    *tls.Config

	// These are the reads end of some pipes whose data we'll shuttle over
	// the RPC channel to the client so it can consume our raw output.
	Stdout, Stderr io.Reader

	// Done is a callback for the server to call when it's been instructed
	// by the client to exit and it's finished all requests in progress.
	Done func()

	Tracer *plugintrace.ServerTracer

	grpcServer *grpc.Server
}

func (s *serverGRPC) Init() error {
	s.grpcServer = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(s.TLS)),
	)

	// Register the health service
	// This is mandatory because clients use it to detect unresponsive servers.
	healthCheck := health.NewServer()
	healthCheck.SetServingStatus(grpcServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.grpcServer, healthCheck)

	// Let the caller's own service register itself
	err := s.Server.RegisterServer(s.grpcServer)
	if err != nil {
		return fmt.Errorf("failed to register server: %s", err)
	}

	return nil
}

func (s *serverGRPC) Serve(l net.Listener) {
	defer s.Done()
	err := s.grpcServer.Serve(l)
	if err != nil && s.Tracer.GRPCServeError != nil {
		s.Tracer.GRPCServeError(err)
	}
}
