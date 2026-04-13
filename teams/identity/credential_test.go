package identity

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewVerifiableCredential(t *testing.T) {
	subject := map[string]any{
		"id":   "did:key:z123",
		"name": "Test Agent",
	}

	cred, err := NewVerifiableCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"TestCredential",
		subject,
	)
	if err != nil {
		t.Fatalf("NewVerifiableCredential() error = %v", err)
	}

	if cred.ID != "urn:uuid:123" {
		t.Errorf("ID = %q, want 'urn:uuid:123'", cred.ID)
	}
	if cred.Issuer != "did:key:issuer" {
		t.Errorf("Issuer = %q, want 'did:key:issuer'", cred.Issuer)
	}
	if len(cred.Type) != 2 {
		t.Errorf("expected 2 types, got %d", len(cred.Type))
	}
	if !cred.HasType(TypeVerifiableCredential) {
		t.Error("expected to have VerifiableCredential type")
	}
	if !cred.HasType("TestCredential") {
		t.Error("expected to have TestCredential type")
	}
}

func TestVerifiableCredential_Validate(t *testing.T) {
	validCred, _ := NewVerifiableCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"TestCredential",
		map[string]any{"id": "test"},
	)

	tests := []struct {
		name    string
		cred    *VerifiableCredential
		wantErr bool
	}{
		{
			name:    "valid credential",
			cred:    validCred,
			wantErr: false,
		},
		{
			name: "missing context",
			cred: &VerifiableCredential{
				ID:                "urn:uuid:123",
				Type:              []string{TypeVerifiableCredential},
				Issuer:            "did:key:issuer",
				IssuanceDate:      time.Now(),
				CredentialSubject: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "missing id",
			cred: &VerifiableCredential{
				Context:           []string{ContextVC},
				Type:              []string{TypeVerifiableCredential},
				Issuer:            "did:key:issuer",
				IssuanceDate:      time.Now(),
				CredentialSubject: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "missing type",
			cred: &VerifiableCredential{
				Context:           []string{ContextVC},
				ID:                "urn:uuid:123",
				Issuer:            "did:key:issuer",
				IssuanceDate:      time.Now(),
				CredentialSubject: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
		{
			name: "missing issuer",
			cred: &VerifiableCredential{
				Context:           []string{ContextVC},
				ID:                "urn:uuid:123",
				Type:              []string{TypeVerifiableCredential},
				IssuanceDate:      time.Now(),
				CredentialSubject: json.RawMessage(`{}`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cred.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifiableCredential_IsExpired(t *testing.T) {
	cred, _ := NewVerifiableCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"TestCredential",
		map[string]any{"id": "test"},
	)

	// No expiration
	if cred.IsExpired() {
		t.Error("expected non-expired credential")
	}

	// Future expiration
	future := time.Now().Add(time.Hour)
	cred = cred.WithExpiration(future)
	if cred.IsExpired() {
		t.Error("expected non-expired credential with future expiration")
	}

	// Past expiration
	past := time.Now().Add(-time.Hour)
	cred = cred.WithExpiration(past)
	if !cred.IsExpired() {
		t.Error("expected expired credential with past expiration")
	}
}

func TestVerifiableCredential_GetSetSubject(t *testing.T) {
	subject := map[string]any{
		"id":   "did:key:z123",
		"name": "Test Agent",
	}

	cred, err := NewVerifiableCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"TestCredential",
		subject,
	)
	if err != nil {
		t.Fatalf("NewVerifiableCredential() error = %v", err)
	}

	// Get subject
	var retrieved map[string]any
	if err := cred.GetSubject(&retrieved); err != nil {
		t.Fatalf("GetSubject() error = %v", err)
	}

	if retrieved["id"] != "did:key:z123" {
		t.Errorf("subject id = %v, want 'did:key:z123'", retrieved["id"])
	}

	// Set new subject
	newSubject := map[string]any{"id": "did:key:z456"}
	if err := cred.SetSubject(newSubject); err != nil {
		t.Fatalf("SetSubject() error = %v", err)
	}

	if err := cred.GetSubject(&retrieved); err != nil {
		t.Fatalf("GetSubject() after set error = %v", err)
	}
	if retrieved["id"] != "did:key:z456" {
		t.Errorf("subject id = %v, want 'did:key:z456'", retrieved["id"])
	}
}

func TestNewAgentCapabilityCredential(t *testing.T) {
	cred, err := NewAgentCapabilityCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"did:key:agent",
		"code-review",
		0.95,
	)
	if err != nil {
		t.Fatalf("NewAgentCapabilityCredential() error = %v", err)
	}

	if !cred.HasType(TypeAgentCapabilityCredential) {
		t.Error("expected AgentCapabilityCredential type")
	}

	var subject AgentCapabilitySubject
	if err := cred.GetSubject(&subject); err != nil {
		t.Fatalf("GetSubject() error = %v", err)
	}

	if subject.ID != "did:key:agent" {
		t.Errorf("subject ID = %q, want 'did:key:agent'", subject.ID)
	}
	if subject.Capability != "code-review" {
		t.Errorf("subject Capability = %q, want 'code-review'", subject.Capability)
	}
	if subject.Confidence != 0.95 {
		t.Errorf("subject Confidence = %f, want 0.95", subject.Confidence)
	}
}

func TestNewAgentDelegationCredential(t *testing.T) {
	cred, err := NewAgentDelegationCredential(
		"urn:uuid:123",
		"did:key:issuer",
		"did:key:delegate",
		"did:key:delegator",
		[]string{"code-review", "documentation"},
	)
	if err != nil {
		t.Fatalf("NewAgentDelegationCredential() error = %v", err)
	}

	if !cred.HasType(TypeAgentDelegationCredential) {
		t.Error("expected AgentDelegationCredential type")
	}

	var subject AgentDelegationSubject
	if err := cred.GetSubject(&subject); err != nil {
		t.Fatalf("GetSubject() error = %v", err)
	}

	if subject.ID != "did:key:delegate" {
		t.Errorf("subject ID = %q, want 'did:key:delegate'", subject.ID)
	}
	if subject.Delegator != "did:key:delegator" {
		t.Errorf("subject Delegator = %q, want 'did:key:delegator'", subject.Delegator)
	}
	if len(subject.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(subject.Capabilities))
	}
}
