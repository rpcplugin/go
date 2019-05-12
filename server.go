package rpcplugin

import (
	"google.golang.org/grpc"
)

// Server is the interface to implement to write a plugin server, which is the
// plugin that is launched by the client and recieves requests from it.
type Server interface {
	RegisterServer(*grpc.Server) error
}

// ServerFunc is a function type that implements interface Server
type ServerFunc func(*grpc.Server) error

var _ Server = ServerFunc(nil)

func (fn ServerFunc) RegisterServer(srv *grpc.Server) error {
	return fn(srv)
}
