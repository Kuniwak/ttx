package slogtest

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

type TestHandler struct {
	t  *testing.T
	sb *strings.Builder
}

func NewTestHandler(t *testing.T) *TestHandler {
	return &TestHandler{t: t, sb: &strings.Builder{}}
}

var _ slog.Handler = &TestHandler{}

func (h *TestHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *TestHandler) Handle(_ context.Context, record slog.Record) error {
	h.sb.Reset()

	h.sb.WriteString(record.Level.String())
	h.sb.WriteString(": ")
	h.sb.WriteString(record.Message)

	if record.NumAttrs() > 0 {
		h.sb.WriteString(": ")
		first := true
		record.Attrs(func(attr slog.Attr) bool {
			if !first {
				h.sb.WriteString(", ")
			}
			first = false
			h.sb.WriteString(attr.Key)
			h.sb.WriteString("=")
			h.sb.WriteString(attr.Value.String())
			return true
		})
	}

	h.sb.WriteString("\n")

	h.t.Log(h.sb.String())
	return nil
}

func (h *TestHandler) WithAttrs([]slog.Attr) slog.Handler {
	panic("not implemented")
}

func (h *TestHandler) WithGroup(string) slog.Handler {
	panic("not implemented")
}

type FuzzHandler struct {
	t  *testing.F
	sb *strings.Builder
}

func NewFuzzHandler(t *testing.F) *FuzzHandler {
	return &FuzzHandler{t: t, sb: &strings.Builder{}}
}

var _ slog.Handler = &FuzzHandler{}

func (h *FuzzHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *FuzzHandler) Handle(_ context.Context, record slog.Record) error {
	h.sb.Reset()
	h.sb.WriteString(record.Level.String())
	h.sb.WriteString(": ")
	h.sb.WriteString(record.Message)
	if record.NumAttrs() > 0 {
		h.sb.WriteString(": ")
		first := true
		record.Attrs(func(attr slog.Attr) bool {
			if !first {
				h.sb.WriteString(", ")
			}
			first = false
			h.sb.WriteString(attr.Key)
			h.sb.WriteString("=")
			h.sb.WriteString(attr.Value.String())
			return true
		})
	}
	h.sb.WriteString("\n")

	h.t.Log(h.sb.String())
	return nil
}

func (h *FuzzHandler) WithAttrs([]slog.Attr) slog.Handler {
	panic("not implemented")
}

func (h *FuzzHandler) WithGroup(string) slog.Handler {
	panic("not implemented")
}
