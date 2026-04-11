package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	h := &DefaultHandler{}

	if err := h.Handle(ctx, "startup"); err != nil {
		log.Fatalf("handler error: %v", err)
	}

	fmt.Fprintln(os.Stdout, "fixture-project running")
	<-ctx.Done()
}
