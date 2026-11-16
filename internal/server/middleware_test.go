package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/VihangaFTW/catchup-mcp/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArgsAttrText(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"flat", `{"include_archived":true,"class":"Physics B"}`, `args.class="Physics B" args.include_archived=true`},
		{"nested", `{"filter":{"state":"active"},"limit":5}`, `args.filter.state=active args.limit=5`},
		{"large id", `{"course_id":123456789012345678}`, `args.course_id=123456789012345678`},
		{"not an object", `[1,2]`, `args=[1,2]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(slog.NewTextHandler(&buf, nil)).Info("x", argsAttr(json.RawMessage(tt.raw)))

			if got := buf.String(); !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestArgsAttrJSON(t *testing.T) {
	var buf bytes.Buffer
	raw := json.RawMessage(`{"include_archived":true,"filter":{"state":"active"}}`)
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", argsAttr(raw))

	want := `"args":{"filter":{"state":"active"},"include_archived":true}`
	if got := buf.String(); !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
}

func TestMCPRequestLogger(t *testing.T) {
	toolCall := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "list_classes",
		Arguments: json.RawMessage(`{"include_archived":true}`),
	}}

	tests := []struct {
		name    string
		method  string
		req     mcp.Request
		res     mcp.Result
		err     error
		want    []string
		notWant []string
	}{
		{
			name:    "other method succeeds",
			method:  "tools/list",
			req:     &mcp.ListToolsRequest{},
			res:     &mcp.ListToolsResult{},
			want:    []string{"level=DEBUG", "method=tools/list"},
			notWant: []string{"err=", "is_error", "call="},
		},
		{
			name:    "tool call succeeds",
			method:  methodCallTool,
			req:     toolCall,
			res:     &mcp.CallToolResult{},
			want:    []string{"level=INFO", "tool=list_classes", "call=", "args.include_archived=true"},
			notWant: []string{"err=", "is_error"},
		},
		{
			name:    "tool call returns an error result",
			method:  methodCallTool,
			req:     toolCall,
			res:     &mcp.CallToolResult{IsError: true},
			want:    []string{"level=WARN", "is_error=true", "tool=list_classes"},
			notWant: []string{"err="},
		},
		{
			name:    "request fails with a protocol error",
			method:  methodCallTool,
			req:     toolCall,
			err:     errors.New("unknown tool"),
			want:    []string{"level=WARN", `err="unknown tool"`},
			notWant: []string{"is_error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			prev := slog.Default()
			slog.SetDefault(logging.NewLogger(&buf, slog.LevelDebug, logging.FormatText))
			t.Cleanup(func() { slog.SetDefault(prev) })

			next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
				return tt.res, tt.err
			}

			res, err := MCPRequestLogger(next)(context.Background(), tt.method, tt.req)

			if res != tt.res || err != tt.err {
				t.Errorf("the logger changed the result: got (%v, %v), want (%v, %v)", res, err, tt.res, tt.err)
			}

			got := buf.String()

			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("got %q, want it to contain %q", got, w)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("got %q, want it not to contain %q", got, nw)
				}
			}
		})
	}
}
