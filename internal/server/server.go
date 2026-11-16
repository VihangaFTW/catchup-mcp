package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/VihangaFTW/catchup-mcp/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ListenAddr = "127.0.0.1:8080"
	MCPPath    = "/mcp"
	Version    = "v0.1.0"
)

// NewHTTPServer wraps a MCP server with the given tools.
//
// A MCP server holds tools, prompts, resources and answers JSON-RPC messages.
// However, it does not implement http.Handler for HTTP communication.
// Hence, the MCP server is wrapped by a local HTTP server.
func NewHTTPServer(tk *Toolkit) *http.Server {
	return &http.Server{
		Addr:              ListenAddr,
		Handler:           newMCPHandler(newMCPServer(tk)),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}

}

func newMCPHandler(server *mcp.Server) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		PropagateRequestCancellation: true,
		Logger:                       mcpSDKLogger(),
	})

	mux := http.NewServeMux()
	// wrap our mcp handler with a default handler that checks for CSRF attacks
	mux.Handle(MCPPath, http.NewCrossOriginProtection().Handler(handler))

	return HTTPRequestLogger(mux)
}

func newMCPServer(tk *Toolkit) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "catchup", Version: Version}, &mcp.ServerOptions{
		Logger:       mcpSDKLogger(),
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})

	mcp.AddTool(server, &mcp.Tool{Name: "list_classes", Description: "List the user's Google Classroom classes, with each class's name, section and id. Archived classes appear when include_archived is true"}, tk.listCourses)

	// logging middleware for mcp requests
	server.AddReceivingMiddleware(MCPRequestLogger)

	return server
}

func mcpSDKLogger() *slog.Logger {
	return logging.ToMinLogger(slog.Default(), slog.LevelWarn).With("component", "mcp-sdk")
}
