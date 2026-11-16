package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/VihangaFTW/catchup-mcp/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const methodCallTool = "tools/call"

// callSeq counts the tool calls (tools/call) this process has received.
// Each call gets a unique sequence number from callSeq.Add(1), which
// links its log records together. One counter is shared by all
// sessions, and it is safe for concurrent use.
var callSeq atomic.Uint64

func HTTPRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}

		start := time.Now()

		next.ServeHTTP(rec, r)

		// statuses < 400 are all debug logs
		// as the mcp logger repeats these logs as info
		level := slog.LevelDebug

		switch {
		// statuses >= 500 are error logs
		case rec.Status() >= http.StatusInternalServerError:
			level = slog.LevelError
		// statuses  >= 400 are warn logs
		case rec.Status() >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		slog.Log(r.Context(), level, "http request finished",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.Status(),
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
			"origin", r.Header.Get("Origin"),
		)
	})
}

func MCPRequestLogger(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		level := slog.LevelDebug

		attrs := []slog.Attr{slog.String("method", method)}

		if method == methodCallTool {
			level = slog.LevelInfo

			attrs = append(attrs, slog.Uint64("call", callSeq.Add(1)))

			// log tool name and arguments passed
			if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				attrs = append(attrs, slog.String("tool", params.Name))

				if len(params.Arguments) > 0 {
					attrs = append(attrs, argsAttr(params.Arguments))
				}
			}
		}

		ctx = logging.CtxWithAttrs(ctx, attrs...)

		start := time.Now()

		res, err := next(ctx, method, req)

		done := []any{slog.Duration("duration", time.Since(start))}

		switch toolRes, _ := res.(*mcp.CallToolResult); {
		// this branch only fires for protocol errors such as unknow tool calls
		case err != nil:
			level = slog.LevelWarn
			done = append(done, slog.Any("err", err))
		// tool ran but could not complete successfully
		case toolRes != nil && toolRes.IsError:
			level = slog.LevelWarn
			done = append(done, slog.Bool("is_error", true))
		}

		slog.Log(ctx, level, "mcp request finished", done...)

		return res, err
	}
}

// statusRecorder embeds a http.ResponseWriter to record
// the response's status for the associated request's log record.
//
// It is required because there is no way to access the status code
// after it's written to the response header.
type statusRecorder struct {
	http.ResponseWriter
	// status should not be accessed directly.
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}

	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Status returns the response status.
// Always prefer this method over direct field access.
func (r *statusRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// argsAttr turns a tool's raw JSON arguments into an "args" group, so that
// each argument is its own log field: args.include_archived=true in text
// logs and a nested object in JSON logs.
//
// If the arguments are not a JSON object, the attribute holds the raw text.
func argsAttr(raw json.RawMessage) slog.Attr {
	dec := json.NewDecoder(bytes.NewReader(raw))
	// keeps large numbers exact without rounding up
	dec.UseNumber()

	var obj map[string]any

	// decode fails, fallback to plaintext json representation
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return slog.String("args", string(raw))
	}

	return slog.Attr{Key: "args",
		Value: slog.GroupValue(objectAttrs(obj)...),
	}

}

// objectAttrs converts a decoded JSON object into attributes sorted by key.
func objectAttrs(obj map[string]any) []slog.Attr {

	// an empty slice with room for one attribute per key
	attrs := make([]slog.Attr, 0, len(obj))

	for _, key := range slices.Sorted(maps.Keys(obj)) {
		// recursively extract each nested key-value pair
		// and wrap all in one group
		if nested, ok := obj[key].(map[string]any); ok {
			attrs = append(attrs, slog.Attr{Key: key, Value: slog.GroupValue(objectAttrs(nested)...)})
		} else {
			attrs = append(attrs, slog.Any(key, obj[key]))
		}
	}

	return attrs
}
