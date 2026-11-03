package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	err := newServer().Run(context.Background(), &mcp.StdioTransport{})

	if err != nil {
		log.Fatal(err)
	}

}
