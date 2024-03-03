package rpcplugin

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"go.rpcplugin.org/rpcplugin/internal/gopluginshim"
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
	Server ServerVersion
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

func (s *serverGRPC) Init(goPluginClose func()) error {
	var opts []grpc.ServerOption
	if s.TLS != nil {
		opts = []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(s.TLS)),
		}
	}
	s.grpcServer = grpc.NewServer(opts...)

	// Register the health service
	// This is mandatory because clients use it to detect unresponsive servers.
	healthCheck := health.NewServer()
	healthCheck.SetServingStatus(grpcServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.grpcServer, healthCheck)

	// If we think we're running as a client of go-plugin rather than a
	// true rpcplugin implementation then we'll implement go-plugin's
	// extra "shutdown" service, since otherwise go-plugin will hang for
	// 2 seconds when it tries to shut this server down.
	// (rpcplugin clients just use a signal for this, rather than a special
	// gRPC service)
	if goPluginClose != nil {
		gopluginshim.RegisterGoPluginShutdown(s.grpcServer, goPluginClose)
	}

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
