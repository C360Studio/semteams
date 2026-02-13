// Package main provides the standalone mock OpenAI server for E2E testing.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/c360studio/semstreams/test/e2e/mock"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)

	server := mock.NewOpenAIServer().
		WithToolArgs("query_entity", `{"entity_id": "c360.logistics.environmental.sensor.temperature.temp-sensor-001"}`).
		// Return JSON format for workflow condition evaluation
		WithCompletionContent(`{"valid": true, "summary": "Analysis complete. Temperature sensor reading exceeds threshold. Recommend monitoring HVAC system."}`)

	if err := server.Start(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Mock OpenAI server listening on %s", server.URL())
	log.Printf("Health endpoint: %s/health", server.URL())
	log.Printf("Chat completions: %s/v1/chat/completions", server.URL())

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := server.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
