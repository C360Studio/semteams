package main

import (
	"context"
	"fmt"
)

// Handler processes messages received by the fixture service.
type Handler interface {
	Handle(ctx context.Context, msg string) error
	Name() string
}

// DefaultHandler is the standard implementation of Handler.
type DefaultHandler struct {
	prefix string
}

// Handle processes the given message and writes output to stdout.
func (h *DefaultHandler) Handle(ctx context.Context, msg string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	label := h.Name()
	fmt.Printf("[%s] %s\n", label, msg)

	return nil
}

// Name returns the handler's identifier label.
func (h *DefaultHandler) Name() string {
	if h.prefix != "" {
		return h.prefix
	}

	return "default"
}

// NewHandler constructs a DefaultHandler with the given prefix.
func NewHandler(prefix string) Handler {
	return &DefaultHandler{prefix: prefix}
}
