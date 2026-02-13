// Package main provides the standalone mock servers for E2E testing.
// It runs both an OpenAI mock server and an AGNTCY mock server.
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
	port := flag.Int("port", 8080, "Port for OpenAI mock server")
	agntcyPort := flag.Int("agntcy-port", 8081, "Port for AGNTCY mock server")
	flag.Parse()

	// Start OpenAI mock server
	openaiAddr := fmt.Sprintf(":%d", *port)
	openaiServer := mock.NewOpenAIServer().
		WithToolArgs("query_entity", `{"entity_id": "c360.logistics.environmental.sensor.temperature.temp-sensor-001"}`).
		// Return JSON format for workflow condition evaluation
		WithCompletionContent(`{"valid": true, "summary": "Analysis complete. Temperature sensor reading exceeds threshold. Recommend monitoring HVAC system."}`)

	if err := openaiServer.Start(openaiAddr); err != nil {
		log.Fatalf("Failed to start OpenAI mock server: %v", err)
	}
	log.Printf("Mock OpenAI server listening on %s", openaiServer.URL())
	log.Printf("  Health endpoint: %s/health", openaiServer.URL())
	log.Printf("  Chat completions: %s/v1/chat/completions", openaiServer.URL())

	// Start AGNTCY mock server
	agntcyAddr := fmt.Sprintf(":%d", *agntcyPort)
	agntcyServer := mock.NewAGNTCYServer()

	if err := agntcyServer.Start(agntcyAddr); err != nil {
		log.Fatalf("Failed to start AGNTCY mock server: %v", err)
	}
	log.Printf("Mock AGNTCY server listening on %s", agntcyServer.URL())
	log.Printf("  Directory: %s/v1/agents/register", agntcyServer.URL())
	log.Printf("  OTEL HTTP: %s/v1/traces", agntcyServer.URL())

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := openaiServer.Stop(); err != nil {
		log.Printf("Error stopping OpenAI mock: %v", err)
	}
	if err := agntcyServer.Stop(); err != nil {
		log.Printf("Error stopping AGNTCY mock: %v", err)
	}
}
