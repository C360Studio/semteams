package identity

import (
	"context"
	"testing"
)

func TestLocalProvider_CreateIdentity(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()
	identity, err := provider.CreateIdentity(ctx, CreateIdentityOptions{
		DisplayName:         "Test Agent",
		InternalRole:        "tester",
		InitialCapabilities: []string{"testing", "validation"},
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}

	if identity.DisplayName != "Test Agent" {
		t.Errorf("DisplayName = %q, want 'Test Agent'", identity.DisplayName)
	}
	if identity.InternalRole != "tester" {
		t.Errorf("InternalRole = %q, want 'tester'", identity.InternalRole)
	}
	if identity.DID.Method != MethodKey {
		t.Errorf("DID.Method = %q, want %q", identity.DID.Method, MethodKey)
	}
	if len(identity.Credentials) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(identity.Credentials))
	}
}

func TestLocalProvider_CreateIdentity_Validation(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()
	_, err = provider.CreateIdentity(ctx, CreateIdentityOptions{
		// Missing DisplayName
	})
	if err == nil {
		t.Error("expected error for missing DisplayName")
	}
}

func TestLocalProvider_ResolveIdentity(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()

	// Create identity
	created, err := provider.CreateIdentity(ctx, CreateIdentityOptions{
		DisplayName: "Test Agent",
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}

	// Resolve identity
	resolved, err := provider.ResolveIdentity(ctx, created.DID)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}

	if resolved.DisplayName != created.DisplayName {
		t.Errorf("DisplayName = %q, want %q", resolved.DisplayName, created.DisplayName)
	}

	// Resolve non-existent
	_, err = provider.ResolveIdentity(ctx, DID{Method: "key", ID: "nonexistent"})
	if err != ErrIdentityNotFound {
		t.Errorf("expected ErrIdentityNotFound, got %v", err)
	}
}

func TestLocalProvider_IssueCredential(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()
	subjectDID := DID{Method: "key", ID: "z123"}

	cred, err := provider.IssueCredential(ctx, subjectDID, TypeAgentCapabilityCredential, AgentCapabilitySubject{
		ID:         subjectDID.String(),
		Capability: "testing",
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("IssueCredential() error = %v", err)
	}

	if cred.Issuer != provider.GetIssuerDID().String() {
		t.Errorf("Issuer = %q, want %q", cred.Issuer, provider.GetIssuerDID().String())
	}
	if cred.Proof == nil {
		t.Error("expected credential to have proof")
	}
}

func TestLocalProvider_VerifyCredential(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()
	subjectDID := DID{Method: "key", ID: "z123"}

	// Issue credential
	cred, err := provider.IssueCredential(ctx, subjectDID, TypeAgentCapabilityCredential, AgentCapabilitySubject{
		ID:         subjectDID.String(),
		Capability: "testing",
	})
	if err != nil {
		t.Fatalf("IssueCredential() error = %v", err)
	}

	// Verify credential
	valid, err := provider.VerifyCredential(ctx, cred)
	if err != nil {
		t.Fatalf("VerifyCredential() error = %v", err)
	}
	if !valid {
		t.Error("expected credential to be valid")
	}
}

func TestLocalProvider_UpdateIdentity(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()

	// Create identity
	identity, err := provider.CreateIdentity(ctx, CreateIdentityOptions{
		DisplayName: "Test Agent",
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}

	// Update identity
	identity.DisplayName = "Updated Agent"
	if err := provider.UpdateIdentity(ctx, identity); err != nil {
		t.Fatalf("UpdateIdentity() error = %v", err)
	}

	// Verify update
	resolved, err := provider.ResolveIdentity(ctx, identity.DID)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if resolved.DisplayName != "Updated Agent" {
		t.Errorf("DisplayName = %q, want 'Updated Agent'", resolved.DisplayName)
	}
}

func TestLocalProvider_DeleteIdentity(t *testing.T) {
	provider, err := NewLocalProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}

	ctx := context.Background()

	// Create identity
	identity, err := provider.CreateIdentity(ctx, CreateIdentityOptions{
		DisplayName: "Test Agent",
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}

	// Delete identity
	if err := provider.DeleteIdentity(ctx, identity.DID); err != nil {
		t.Fatalf("DeleteIdentity() error = %v", err)
	}

	// Verify deletion
	_, err = provider.ResolveIdentity(ctx, identity.DID)
	if err != ErrIdentityNotFound {
		t.Errorf("expected ErrIdentityNotFound, got %v", err)
	}

	// Delete non-existent
	err = provider.DeleteIdentity(ctx, DID{Method: "key", ID: "nonexistent"})
	if err != ErrIdentityNotFound {
		t.Errorf("expected ErrIdentityNotFound, got %v", err)
	}
}

func TestDefaultProviderFactory(t *testing.T) {
	// Local provider
	provider, err := DefaultProviderFactory(ProviderConfig{
		ProviderType: "local",
	})
	if err != nil {
		t.Fatalf("DefaultProviderFactory(local) error = %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}

	// Default (empty type) = local
	provider, err = DefaultProviderFactory(ProviderConfig{})
	if err != nil {
		t.Fatalf("DefaultProviderFactory(empty) error = %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider for empty type")
	}

	// Unknown type
	_, err = DefaultProviderFactory(ProviderConfig{
		ProviderType: "unknown",
	})
	if err != ErrUnknownProviderType {
		t.Errorf("expected ErrUnknownProviderType, got %v", err)
	}
}
