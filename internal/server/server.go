package server

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ListenAddr = "127.0.0.1:8080"
	MCPPath    = "/mcp"
)

// NewHTTPServer creates a MCP server with the given tools.
// It holds tools, prompts, resources and answers JSON-RPC messages.
// However, it does not implement http.Handler for HTTP communication.
// Hence, the MCP server is wrapped by a local HTTP server, listening on port 8080.
func NewHTTPServer(tk *Toolkit) *http.Server {
	mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "catchup", Version: "v1.0.0"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})

	mcp.AddTool(mcpSrv, &mcp.Tool{Name: "list_classes", Description: "List the user's Google Classroom classes, with each class's name, section and id. Archived classes appear when include_archived is true"}, tk.listCourses)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpSrv }, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// cancel handler ctx when the client disconnects
		PropagateRequestCancellation: true,
	})

	mux := http.NewServeMux()
	mux.Handle(MCPPath, http.NewCrossOriginProtection().Handler(handler))

	httpSrv := &http.Server{
		Addr:              ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return httpSrv
}
