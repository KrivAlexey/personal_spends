// Package logging builds the service's slog logger and carries per-request
// attributes in the context (ADR 0013).
//
// Only three attributes travel this way: request_id (set by the HTTP
// middleware) and batch/batches (set by the importer around each batch). Every
// record logged with a …Context method gets them, from any layer.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
)

// New returns a logger writing records at or above level ("debug", "info",
// "warn", "error", case-insensitive; "" = info) in format ("text" or "json";
// "" = text). An unknown value is an error naming the variable it came from.
func New(w io.Writer, level, format string) (*slog.Logger, error) {
	lvl := slog.LevelInfo
	if level != "" {
		if err := lvl.UnmarshalText([]byte(level)); err != nil {
			return nil, fmt.Errorf("logging: invalid LOG_LEVEL %q", level)
		}
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch format {
	case "", "text":
		h = slog.NewTextHandler(w, opts)
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		return nil, fmt.Errorf("logging: invalid LOG_FORMAT %q", format)
	}
	return slog.New(NewHandler(h)), nil
}

type ctxKey struct{}

// WithAttrs returns a copy of ctx holding attrs on top of the ones it already
// holds. An attr whose key is already there replaces it.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	merged := slices.Clone(fromContext(ctx))
	for _, a := range attrs {
		i := slices.IndexFunc(merged, func(m slog.Attr) bool { return m.Key == a.Key })
		if i >= 0 {
			merged[i] = a
		} else {
			merged = append(merged, a)
		}
	}
	return context.WithValue(ctx, ctxKey{}, merged)
}

func fromContext(ctx context.Context) []slog.Attr {
	attrs, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	return attrs
}

// NewHandler wraps h so that every record logged with a context also gets the
// attributes stored in it by WithAttrs.
func NewHandler(h slog.Handler) slog.Handler {
	return ctxHandler{inner: h}
}

type ctxHandler struct {
	inner slog.Handler
}

func (h ctxHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs := fromContext(ctx); len(attrs) > 0 {
		r = r.Clone()
		r.AddAttrs(attrs...)
	}
	return h.inner.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{inner: h.inner.WithGroup(name)}
}
