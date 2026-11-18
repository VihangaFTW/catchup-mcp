package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/VihangaFTW/catchup-mcp/internal/auth"
	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
	"github.com/VihangaFTW/catchup-mcp/internal/config"
	"github.com/VihangaFTW/catchup-mcp/internal/google"
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

	inScope, err := tk.Classroom.ListCourses(ctx, role, states)
	if err != nil {
		return failureToResponse(ctx, "list your classes", err)
	}

	other, err := tk.Classroom.ListCourses(ctx, role.Other(), states)
	if err != nil {
		return failureToResponse(ctx, "list your classes", err)
	}

	var b strings.Builder

	b.WriteString(today(tk.Now(), tk.Config.Location))
	b.WriteString("\n\n")

	if len(inScope) == 0 {
		fmt.Fprintf(&b, "You have no classes as a %s.\n", role)
	} else {
		fmt.Fprintf(&b, "Classes where you are a %s: %d\n", role, len(inScope))
		for _, c := range inScope {
			fmt.Fprintf(&b, "- %s\n", c.Label())
		}
	}

	if len(other) > 0 {
		fmt.Fprintf(&b, "\nClasses where you are a %s: %d. The tools in %s role do not cover them.\n", role.Other(), len(other), tk.Config.Role)

		for _, c := range other {
			fmt.Fprintf(&b, "- %s\n", c.Label())
		}
	}

	slog.DebugContext(ctx, "listed classes", "role", role, "in_scope", len(inScope), "other_role", len(other), "include_archived", args.IncludeArchived)

	return toTextResult(b.String()), nil, nil

}

type ListTopicsArgs struct {
	Course string `json:"course" jsonschema:"the course id, or words from the course name or section, such as 'physics a2'"`
}

func (tk *Toolkit) listTopics(ctx context.Context, req *mcp.CallToolRequest, args ListTopicsArgs) (*mcp.CallToolResult, any, error) {

	const action = "list the topics"

	course, res, err := tk.resolveClass(ctx, args.Course)

	if err != nil {
		return failureToResponse(ctx, action, err)
	}

	if res != nil {
		return res, nil, nil
	}

	topics, err := tk.Classroom.ListTopics(ctx, course.ID)

	if err != nil {
		return failureToResponse(ctx, action, err)
	}

	var b strings.Builder

	b.WriteString(today(tk.Now(), tk.Config.Location))
	b.WriteString("\n\n")

	if len(topics) == 0 {
		fmt.Fprintf(&b, "%s has no topics.\n", course.Label())
	} else {
		fmt.Fprintf(&b, "Topics in %s: %d\n", course.Label(), len(topics))
		for _, t := range topics {
			fmt.Fprintf(&b, "- %s\n", t.Label())
		}
	}

	slog.DebugContext(ctx, "listed topics", "course", course.ID, "topics", len(topics))

	return toTextResult(b.String()), nil, nil
}

// resolveClass finds the one class that query names, among the classes that
// the mode covers. The query is a class id, or words from the name and the
// section.
//
// When several classes match, the active ones win. When the query still matches
// no class or several classes, the second return value is an error result that
// lists the choices.
func (tk *Toolkit) resolveClass(ctx context.Context, query string) (classroom.Course, *mcp.CallToolResult, error) {
	states := []classroom.CourseState{classroom.StateActive, classroom.StateArchived}

	courses, err := tk.Classroom.ListCourses(ctx, tk.Config.Role, states)

	if err != nil {
		return classroom.Course{}, nil, err
	}

	matches := classroom.MatchCourses(courses, query)

	// remove archived matches
	if len(matches) > 1 {
		isActive := func(c classroom.Course) bool { return c.State == classroom.StateActive }

		if slices.ContainsFunc(matches, isActive) {
			matches = slices.DeleteFunc(matches, func(c classroom.Course) bool { return !isActive(c) })
		}
	}

	if len(matches) == 1 {
		return matches[0], nil, nil
	}

	// no matching courses
	slog.InfoContext(ctx, "course query did not resolve to one class", "query", query, "matches", len(matches))

	res := toTextResult(classroom.ResolutionMsg(query, matches, courses))
	res.IsError = true

	return classroom.Course{}, res, nil
}

// func (tk *Toolkit) listTopics(ctx context.Context, req *mcp.CallToolRequest, args ListTopicsArgs) (*mcp.CallToolResult, any, error) {

// }

// failureToResponse converts a tool call error into the response that the
// caller can best act on. action is an imperative phrase naming the operation,
// such as "list your classes".
//
// The structured output of the result is always nil.
func failureToResponse(ctx context.Context, action string, err error) (*mcp.CallToolResult, any, error) {

	// a SignInStartedError is a signal that indicates further
	// action from the user: signing in to Google via a URL
	if signErr, ok := errors.AsType[*auth.SignInStartedError](err); ok {
		return toTextResult("Sign-in opened in your browser. Finish it, then ask again. " +
			"If no browser opened, open this link: " + signErr.URL), nil, nil
	}

	// attach advice to results for Google API errors
	if apiErr, ok := errors.AsType[*google.APIError](err); ok {
		// google.Classroom.get already logs the API error, so do not log it again
		res := toTextResult(fmt.Sprintf("Could not %s. %s", action, apiErr.Advice()))
		res.IsError = true
		return res, nil, nil
	}

	// Log error cause here because the SDK turns the returned error
	// into a result with IsError set, so our MCP request logger never sees err
	slog.ErrorContext(ctx, "tool call failed", "action", action, "err", err)

	return nil, nil, fmt.Errorf("could not %s: %w", action, err)

}

func toTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
