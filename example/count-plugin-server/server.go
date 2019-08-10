package main

import (
	"context"
	"log"

	"go.rpcplugin.org/rpcplugin"
	"go.rpcplugin.org/rpcplugin/example/countplugin1"
	"google.golang.org/grpc"
)

// countplugin1Server is the server implementation of the "count" plugin protocol
// version 1. If a version 2 were added later, that'd be a separate type
// countplugin2Server, against a different protobuf package.
type countplugin1Server struct {
	count  int64
	logger *log.Logger
}

// countplugin1Server must implement the RPC server interface
var _ countplugin1.CounterServer = (*countplugin1Server)(nil)

func (s *countplugin1Server) Count(ctx context.Context, req *countplugin1.Count_Request) (*countplugin1.Count_Response, error) {
	s.count++
	s.logger.Printf("Count: incremented the counter to %d", s.count)
	return &countplugin1.Count_Response{}, nil
}

func (s *countplugin1Server) GetCount(ctx context.Context, req *countplugin1.GetCount_Request) (*countplugin1.GetCount_Response, error) {
	s.logger.Printf("GetCount: request for counter value (currently %d)", s.count)
	return &countplugin1.GetCount_Response{
		Count: s.count,
	}, nil
}

// protocolVersion1 is an implementation of rpcplugin.ServerVersion that implements
// protocol version 1.
type protocolVersion1 struct {
	logger *log.Logger
}

// protocolVersion1 must implement the rpcplugin.ServerVersion interface
var _ rpcplugin.ServerVersion = protocolVersion1{}

func (p protocolVersion1) RegisterServer(server *grpc.Server) error {
	countplugin1.RegisterCounterServer(server, &countplugin1Server{logger: p.logger})
	return nil
}
