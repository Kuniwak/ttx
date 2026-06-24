package slograw

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type Handler struct {
	w     io.Writer
	mu    sync.Mutex
	level slog.Level
}

func NewHandler(w io.Writer, level slog.Level) *Handler {
	return &Handler{w: w, level: level}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h.level <= level
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, err := io.WriteString(h.w, record.Level.String()); err != nil {
		return fmt.Errorf("slograw.Handler.Handle: failed to write level: %v", err)
	}
	if _, err := io.WriteString(h.w, ": "); err != nil {
		return fmt.Errorf("slograw.Handler.Handle: failed to write level: %v", err)
	}
	if _, err := io.WriteString(h.w, record.Message); err != nil {
		return fmt.Errorf("slograw.Handler.Handle: failed to write level: %v", err)
	}

	if record.NumAttrs() > 0 {
		if _, err := io.WriteString(h.w, ": "); err != nil {
			return fmt.Errorf("slograw.Handler.Handle: failed to write level: %v", err)
		}

		first := true
		record.Attrs(func(attr slog.Attr) bool {
			if !first {
				if _, err := io.WriteString(h.w, ", "); err != nil {
					return false
				}
			}
			first = false
			if _, err := io.WriteString(h.w, attr.Key); err != nil {
				return false
			}
			if _, err := io.WriteString(h.w, "="); err != nil {
				return false
			}
			if _, err := io.WriteString(h.w, attr.Value.String()); err != nil {
				return false
			}
			return true
		})
	}

	if _, err := io.WriteString(h.w, "\n"); err != nil {
		return fmt.Errorf("slograw.Handler.Handle: failed to write level: %v", err)
	}
	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	panic("not implemented")
}

func (h *Handler) WithGroup(name string) slog.Handler {
	panic("not implemented")
}
