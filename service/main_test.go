//go:build integration

package service

import (
	"log"
	"testing"
	"time"

	"github.com/c360/semstreams/natsclient"
)

// Package-level shared test client to avoid Docker resource exhaustion
var (
	sharedTestClient *natsclient.TestClient
	sharedNATSClient *natsclient.Client
)

// TestMain sets up a single shared NATS container for all service package tests
func TestMain(m *testing.M) {
	// Build tag ensures this only runs with -tags=integration
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create shared test client: %v", err)
	}

	sharedTestClient = testClient
	sharedNATSClient = testClient.Client

	// Run all tests
	exitCode := m.Run()

	// Cleanup
	testClient.Terminate()

	if exitCode != 0 {
		log.Fatal("tests failed")
	}
}

// getSharedTestClient returns the shared test client for tests that need it
func getSharedTestClient(t *testing.T) *natsclient.TestClient {
	if sharedTestClient == nil {
		t.Fatal("Shared test client not initialized - TestMain should have created it")
	}
	return sharedTestClient
}

// getSharedNATSClient returns the shared NATS client for tests that need it
func getSharedNATSClient(t *testing.T) *natsclient.Client {
	if sharedNATSClient == nil {
		t.Fatal("Shared NATS client not initialized - TestMain should have created it")
	}
	return sharedNATSClient
}
