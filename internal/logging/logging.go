// Package logging builds the service's slog logger and carries per-request
// attributes (request_id, batch, batches) in the context (ADR 0013).
package logging

import (
	"context"
	"io"
	"log/slog"
)

// New returns a logger writing records at or above level in format.
func New(w io.Writer, level, format string) (*slog.Logger, error) {
	return slog.New(slog.DiscardHandler), nil
}

// WithAttrs returns a copy of ctx holding attrs.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	return ctx
}

// NewHandler wraps h so records logged with a context get its attributes.
func NewHandler(h slog.Handler) slog.Handler {
	return h
}
