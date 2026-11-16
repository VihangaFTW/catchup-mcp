package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/VihangaFTW/catchup-mcp/internal/auth"
	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
	"github.com/VihangaFTW/catchup-mcp/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Toolkit struct {
	Classroom classroom.Classroom
	Config    config.Config
	Now       Clock
}

type ListClassesArgs struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"also list archived classes"`
}

func (tk *Toolkit) listCourses(ctx context.Context, req *mcp.CallToolRequest, args ListClassesArgs) (*mcp.CallToolResult, any, error) {
	states := []classroom.CourseState{classroom.StateActive}

	if args.IncludeArchived {
		states = append(states, classroom.StateArchived)
	}

	role := tk.Config.Role

	courses, err := tk.Classroom.ListCourses(ctx, role, states)

	if err != nil {
		action := "list your classes"
		return buildResult(ctx, action, err)
	}

	var b strings.Builder

	// timezone metadata
	b.WriteString(today(tk.Now(), tk.Config.Location))
	b.WriteString("\n\n")

	if (len(courses)) == 0 {
		fmt.Fprintf(&b, "You have no classes as a %s.\n", role)
	} else {
		fmt.Fprintf(&b, "Classes where you are a %s: %d\n", role, len(courses))
		for _, c := range courses {
			fmt.Fprintf(&b, "- %s\n", c.Label())
		}
	}

	return textResult(b.String()), nil, nil

}

func buildResult(ctx context.Context, action string, err error) (*mcp.CallToolResult, any, error) {

	// a SignInStartedError is a signal that indicates futher
	// action from user: singing in to google via a URL
	if signErr, ok := errors.AsType[*auth.SignInStartedError](err); ok {
		return textResult("Sign-in opened in your browser. Finish it, then ask again. " +
			"If no browser opened, open this link: " + signErr.URL), nil, nil
	}

	// Log error cause here because the SDK turns the returned error
	// into a result with IsError set, so our MCP request logger never sees err
	slog.ErrorContext(ctx, "tool call failed", "action", action, "err", err)

	return nil, nil, fmt.Errorf("could not %s: %w", action, err)

}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
