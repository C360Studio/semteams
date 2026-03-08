package main

import (
	"context"
	"testing"
)

func TestDefaultHandler_Name(t *testing.T) {
	h := &DefaultHandler{prefix: "test"}

	if got := h.Name(); got != "test" {
		t.Errorf("Name() = %q, want %q", got, "test")
	}
}

func TestDefaultHandler_Name_Default(t *testing.T) {
	h := &DefaultHandler{}

	if got := h.Name(); got != "default" {
		t.Errorf("Name() = %q, want %q", got, "default")
	}
}

func TestDefaultHandler_Handle(t *testing.T) {
	h := NewHandler("test")

	if err := h.Handle(context.Background(), "hello"); err != nil {
		t.Errorf("Handle() unexpected error: %v", err)
	}
}
