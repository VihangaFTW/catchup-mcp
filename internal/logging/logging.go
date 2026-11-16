package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
)

var (
	_ slog.Handler = (*contextHandler)(nil)
	_ slog.Handler = (*minLevelHandler)(nil)
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat attempts to cast s to a Format.
// If s is empty, FormatText is returned.
func ParseFormat(s string) (Format, error) {
	switch f := Format(s); f {
	case "":
		return FormatText, nil
	case FormatText, FormatJSON:
		return f, nil
	default:
		return "", fmt.Errorf("unknown log format %q", s)
	}
}

// ParseLevel attempts to cast s to a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	var level slog.Level

	if s == "" {
		return level, nil
	}

	if err := level.UnmarshalText([]byte(s)); err != nil {
		return level, fmt.Errorf("unknown log level %q", s)
	}

	return level, nil
}

// NewLogger returns a *slog.Logger with the given configuration.
func NewLogger(w io.Writer, level slog.Leveler, format Format) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	var base slog.Handler

	if format == FormatText {
		base = slog.NewTextHandler(w, opts)
	} else {
		base = slog.NewJSONHandler(w, opts)
	}

	return slog.New(&contextHandler{next: base})

}

// ToMinLogger converts a logger to a new logger that
// records logs with a level greater than or equals to min. 
func ToMinLogger(l *slog.Logger, min slog.Level) *slog.Logger {
	return slog.New(&minLevelHandler{min: min, next: l.Handler()})
}

// CtxWithAttrs returns a new context that carries the given log fields.
func CtxWithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)

	// slices.Clip forces a copy of parent slice's array content 
	// into a new array when calling append().
	// This prevents a race condition when multiple child contexts
	// write to a slice from the same parent context and ends up
	// overwriting the array values.
	return context.WithValue(ctx, ctxKey{}, append(slices.Clip(prev), attrs...))
}

// Implementation of logging handler that adds context scoped attributes to logs.

type ctxKey struct{}

// contextHandler takes a log record, adds attributes from a Context,
// and then calls next.Handle() to pass it on.
type contextHandler struct {
	// next formats and writes to stderr.
	next slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		// records share state so copy first
		r = r.Clone()
		r.AddAttrs(attrs...)
	}

	return h.next.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}

// Implementation of logging handler with a minimum log level configuration.

type minLevelHandler struct {
	next slog.Handler
	min  slog.Level
}

func (h *minLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h *minLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}

func (h *minLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &minLevelHandler{next: h.next.WithAttrs(attrs), min: h.min}
}

func (h *minLevelHandler) WithGroup(name string) slog.Handler {
	return &minLevelHandler{next: h.next.WithGroup(name), min: h.min}
}
