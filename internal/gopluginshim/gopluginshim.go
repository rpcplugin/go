package gopluginshim

import (
	"context"

	"google.golang.org/grpc"
)

// RegisterGoPluginShutdown provides a minimal implementation of the extra
// gRPC service that go-plugin client expect to find on any go-plugin server
// to command it to shut down.
//
// Without this, go-plugin clients will hang for two seconds every time they
// try to shut down a plugin.
func RegisterGoPluginShutdown(server *grpc.Server, close func()) {
	RegisterGRPCControllerServer(server, &controllerServer{})
}

type controllerServer struct {
	close func()
}

// Shutdown implements GRPCControllerServer.
func (c *controllerServer) Shutdown(context.Context, *Empty) (*Empty, error) {
	c.close()
	return nil, nil
}
