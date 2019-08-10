package main

import (
	"context"

	"go.rpcplugin.org/rpcplugin"
	"go.rpcplugin.org/rpcplugin/example/countplugin1"
	"google.golang.org/grpc"
)

// countplugin1Server is the server implementation of the "count" plugin protocol
// version 1. If a version 2 were added later, that'd be a separate type
// countplugin2Server, against a different protobuf package.
type countplugin1Server struct {
	count int64
}

// countplugin1Server must implement the server interface
var _ countplugin1.CounterServer = (*countplugin1Server)(nil)

func (s *countplugin1Server) Count(ctx context.Context, req *countplugin1.Count_Request) (*countplugin1.Count_Response, error) {
	s.count++
	return &countplugin1.Count_Response{}, nil
}

func (s *countplugin1Server) GetCount(ctx context.Context, req *countplugin1.GetCount_Request) (*countplugin1.GetCount_Response, error) {
	return &countplugin1.GetCount_Response{
		Count: s.count,
	}, nil
}

// protocolVersion1 is an implementation of rpcplugin.Server that implements
// protocol version 1.
type protocolVersion1 struct{}

var _ rpcplugin.Server = protocolVersion1{}

func (p protocolVersion1) RegisterServer(server *grpc.Server) error {
	countplugin1.RegisterCounterServer(server, &countplugin1Server{})
	return nil
}
