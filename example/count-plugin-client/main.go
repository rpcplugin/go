package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"go.rpcplugin.org/rpcplugin"
	"go.rpcplugin.org/rpcplugin/example/countplugin1"
	"go.rpcplugin.org/rpcplugin/plugintrace"
	"google.golang.org/grpc"
)

func main() {
	logger := log.New(os.Stderr, "client: ", log.Flags())
	ctx := plugintrace.WithClientTracer(context.Background(), plugintrace.ClientLogTracer(logger))

	// We'll start by launching the plugin server. This expects to find
	// the executable "count-plugin-server" in your PATH, which you can
	// achieve by "go install"ing the server package and making sure your
	// GOBIN directory is in your PATH.
	plugin, err := rpcplugin.New(ctx, &rpcplugin.ClientConfig{
		Handshake: rpcplugin.HandshakeConfig{
			// The client and server must both agree on the CookieKey and
			// CookieValue so that the server can detect whether it's running
			// as a child process of its expected client. If not, it will
			// produce an error message an exit immediately.
			CookieKey:   "COUNT_PLUGIN_COOKIE",
			CookieValue: "e8f9c7d7-20fd-55c7-83f9-bee91db2922b",
		},

		ProtoVersions: map[int]rpcplugin.ClientVersion{
			1: protocolVersion1{},
		},

		Cmd:    exec.Command("count-plugin-server"),
		Stderr: os.Stderr, // The two processes can just share our stderr here
	})
	if err != nil {
		logger.Fatalf("failed to start plugin: %s", err)
	}

	// Now we retrieve the client proxy object that we'll use to call
	// RPC functions on the server. clientRaw here is of type interface{}
	// because rpcplugin can't predict what type the result will be, but
	// we know (because we defined the protocol) that protocol version 1
	// is represented by countplugin1.CounterClient.
	//
	// In a real application, you might want to wrap this up in an
	// application-specific wrapper function that has a more precise return
	// type. For example, perhaps your application has its own interface
	// type representing the plugin functionality and one implementation
	// of that interface per protocol version that wraps the raw GRPC client
	// object.
	protoVersion, clientRaw, err := plugin.Client(ctx)
	if err != nil {
		logger.Fatalf("failed to create plugin client: %s", err)
	}
	if protoVersion != 1 {
		logger.Fatalf("server selected unsupported protocol version %d", protoVersion)
	}
	client := clientRaw.(countplugin1.CounterClient)

	// In programs using plugins, we'll generally want to intercept the
	// signals that would normally terminate the program immediately so
	// that we have a chance to shut down the plugins gracefully.
	//
	// This is overkill for this very simple program, but it's included here
	// to show how you might do it in a real program.
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt, os.Kill)

	// In the absense of any _real_ behavior for this example client, we'll
	// just have it increment the counter every two seconds until it's
	// interrupted.
	tick := time.Tick(2 * time.Second)

	logger.Printf("will now increment the counter every two seconds until interrupted")

Events:
	for {
		select {
		case <-tick:
			logger.Printf("incrementing the counter")
			_, err = client.Count(ctx, &countplugin1.Count_Request{})
			if err != nil {
				logger.Printf("call to Count failed: %s", err)
			}
			resp, err := client.GetCount(ctx, &countplugin1.GetCount_Request{})
			if err != nil {
				logger.Printf("call to GetCount failed: %s", err)
			}
			logger.Printf("server reports count is %d", resp.Count)
		case <-interrupt:
			break Events
		}
	}

	// Must be sure to close the plugin when we're finished with it, so we
	// don't leave an orphaned child process behind.
	err = plugin.Close()
	if err != nil {
		logger.Printf("failed to close plugin: %s", err)
	}
}

// protocolVersion1 is an implementation of rpcplugin.ClientVersion that implements
// protocol version 1.
type protocolVersion1 struct{}

// protocolVersion1 must implement the rpcplugin.ClientVersion interface
var _ rpcplugin.ClientVersion = protocolVersion1{}

func (p protocolVersion1) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
	return countplugin1.NewCounterClient(conn), nil
}
