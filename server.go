package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SayHiParams struct {
	Name string `json:"name" jsonschema:"the person's name"`
}

func sayHi(ctx context.Context, req *mcp.CallToolRequest, args SayHiParams) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Hi " + args.Name}}}, nil, nil
}

func newServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, sayHi)
	return server
}
