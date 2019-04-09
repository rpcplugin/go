package rpcplugin

import (
	"context"

	"google.golang.org/grpc"
)

// Client is the interface to implement to write a plugin client, which is the
// host program that launches plugins and initiates requests to them.
type Client interface {
	ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)
}

// Server is the interface to implement to write a plugin server, which is the
// plugin that is launched by the client and recieves requests from it.
type Server interface {
	RegisterServer(*grpc.Server) error
}

// ClientFunc is a function type that implements interface Client
type ClientFunc func(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)

var _ Client = ClientFunc(nil)

func (fn ClientFunc) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
	return fn(ctx, conn)
}

// ServerFunc is a function type that implements interface Server
type ServerFunc func(*grpc.Server) error

var _ Server = ServerFunc(nil)

func (fn ServerFunc) RegisterServer(srv *grpc.Server) error {
	return fn(srv)
}
