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

// ClientFunc is a function type that implements interface Client
type ClientFunc func(ctx context.Context, conn *grpc.ClientConn) (interface{}, error)

var _ Client = ClientFunc(nil)

func (fn ClientFunc) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
	return fn(ctx, conn)
}
