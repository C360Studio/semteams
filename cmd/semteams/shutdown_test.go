package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lifecycleProbe struct {
	signal       chan struct{}
	startContext context.Context
	stopContext  context.Context
	startError   error
	stopError    error
}

func (p *lifecycleProbe) StartAll(ctx context.Context) error {
	p.startContext = ctx
	close(p.signal)
	return p.startError
}
func (p *lifecycleProbe) StopAll(ctx context.Context) error {
	p.stopContext = ctx
	if p.startContext.Err() != nil {
		return errors.New("runtime canceled before controlled shutdown")
	}
	return p.stopError
}
func TestShutdownKeepsStartAuthorityLiveAndBoundsStop(t *testing.T) {
	p := &lifecycleProbe{signal: make(chan struct{})}
	if err := runUntilShutdown(t.Context(), p, p.signal, time.Second); err != nil {
		t.Fatal(err)
	}
	if p.stopContext == nil {
		t.Fatal("services never stopped")
	}
	if _, ok := p.stopContext.Deadline(); !ok {
		t.Fatal("StopAll did not receive the caller shutdown bound")
	}
}
func TestShutdownPropagatesFailure(t *testing.T) {
	want := errors.New("join failed")
	p := &lifecycleProbe{signal: make(chan struct{}), stopError: want}
	if err := runUntilShutdown(t.Context(), p, p.signal, time.Second); !errors.Is(err, want) {
		t.Fatalf("shutdown error = %v", err)
	}
}
func TestStartupFailureDoesNotWaitForSignal(t *testing.T) {
	want := errors.New("readiness refused")
	p := &lifecycleProbe{signal: make(chan struct{}), startError: want}
	if err := runUntilShutdown(t.Context(), p, p.signal, time.Second); !errors.Is(err, want) {
		t.Fatalf("startup error = %v", err)
	}
}
