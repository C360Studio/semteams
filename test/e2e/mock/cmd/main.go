// Package main provides the standalone mock servers for E2E testing.
// It runs OpenAI, AGNTCY, and TrustGraph mock servers.
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
	trustgraphPort := flag.Int("trustgraph-port", 8082, "Port for TrustGraph mock server")
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

	// Start TrustGraph mock server
	trustgraphAddr := fmt.Sprintf(":%d", *trustgraphPort)
	trustgraphServer := mock.NewTrustGraphServer().
		WithImportTriples(getTrustGraphTestTriples()).
		WithRAGResponse("threat", "Based on the knowledge graph, there are 3 active threat indicators in the supply chain. The primary concerns are related to sensor anomalies in Zone 7.").
		WithRAGResponse("sensor", "Temperature Sensor Zone 7 is reporting readings outside normal parameters. Historical data suggests potential equipment degradation.").
		WithDefaultRAGResponse("No relevant information found in the knowledge graph for this query.")

	if err := trustgraphServer.Start(trustgraphAddr); err != nil {
		log.Fatalf("Failed to start TrustGraph mock server: %v", err)
	}
	log.Printf("Mock TrustGraph server listening on %s", trustgraphServer.URL())
	log.Printf("  Triples query: %s/api/v1/triples-query", trustgraphServer.URL())
	log.Printf("  Knowledge: %s/api/v1/knowledge", trustgraphServer.URL())
	log.Printf("  GraphRAG: %s/api/v1/graph-rag", trustgraphServer.URL())
	log.Printf("  Stats: %s/stats", trustgraphServer.URL())

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
	if err := trustgraphServer.Stop(); err != nil {
		log.Printf("Error stopping TrustGraph mock: %v", err)
	}
}

// getTrustGraphTestTriples returns the test triples to seed the TrustGraph mock.
// These triples represent entities that will be imported into SemStreams.
func getTrustGraphTestTriples() []mock.TGTriple {
	return []mock.TGTriple{
		// Threat report entity
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/threat-001"),
			P: mock.NewEntityValue("http://www.w3.org/2000/01/rdf-schema#label"),
			O: mock.NewLiteralValue("Supply Chain Threat Report"),
		},
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/threat-001"),
			P: mock.NewEntityValue("http://www.w3.org/1999/02/22-rdf-syntax-ns#type"),
			O: mock.NewEntityValue("http://trustgraph.ai/vocab/ThreatReport"),
		},
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/threat-001"),
			P: mock.NewEntityValue("http://trustgraph.ai/vocab/severity"),
			O: mock.NewLiteralValue("high"),
		},
		// Sensor metadata entity
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/sensor-zone7"),
			P: mock.NewEntityValue("http://www.w3.org/2000/01/rdf-schema#label"),
			O: mock.NewLiteralValue("Temperature Sensor Zone 7"),
		},
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/sensor-zone7"),
			P: mock.NewEntityValue("http://www.w3.org/1999/02/22-rdf-syntax-ns#type"),
			O: mock.NewEntityValue("http://trustgraph.ai/vocab/Sensor"),
		},
		{
			S: mock.NewEntityValue("http://trustgraph.ai/e/sensor-zone7"),
			P: mock.NewEntityValue("http://trustgraph.ai/vocab/location"),
			O: mock.NewLiteralValue("Zone 7"),
		},
	}
}
