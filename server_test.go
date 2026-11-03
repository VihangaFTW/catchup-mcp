package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGreet(t *testing.T) {
	ctx := context.Background()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// initialize server connection
	serverSession, err := newServer().Connect(ctx, serverTransport, nil)

	if err != nil {
		t.Fatal(err)
	}

	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)

	// initialize client connection
	clientSession, err := client.Connect(ctx, clientTransport, nil)

	if err != nil {
		t.Fatal(err)
	}

	defer clientSession.Close()

	// client calls greet tool
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "greet", Arguments: map[string]any{"name": "vihanga"}})

	if err != nil {
		t.Fatal(err)
	}

	if res.IsError {
		t.Fatalf("tool returned an error: %v", res.Content)
	}

	if size := len(res.Content); size != 1 {
		t.Fatalf("got %d content items, want 1", size)
	}

	// ensure content type is text
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("got content of type %T, want *mcp.TextContent", res.Content[0])
	}

	// verify expected result
	want := "Hi vihanga"
	if tc.Text != want {
		t.Errorf("got %q, want %q", tc.Text, want)
	}

}
