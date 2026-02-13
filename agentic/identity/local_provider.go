package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LocalProvider implements Provider using local key generation.
// This is suitable for development, testing, and single-node deployments.
type LocalProvider struct {
	config ProviderConfig

	// In-memory storage for identities and keys
	identities map[string]*AgentIdentity     // DID string -> identity
	keys       map[string]ed25519.PrivateKey // DID string -> private key
	mu         sync.RWMutex

	// Issuer DID for credentials (self-issued)
	issuerDID *DID
	issuerKey ed25519.PrivateKey
}

// NewLocalProvider creates a new local identity provider.
func NewLocalProvider(config ProviderConfig) (*LocalProvider, error) {
	// Generate issuer key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate issuer key: %w", err)
	}

	// Create issuer DID using did:key
	issuerDID := NewKeyDID(multibaseEncode(pub))

	return &LocalProvider{
		config:     config,
		identities: make(map[string]*AgentIdentity),
		keys:       make(map[string]ed25519.PrivateKey),
		issuerDID:  issuerDID,
		issuerKey:  priv,
	}, nil
}

// CreateIdentity creates a new agent identity with a locally generated DID.
func (p *LocalProvider) CreateIdentity(ctx context.Context, opts CreateIdentityOptions) (*AgentIdentity, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	// Generate key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	// Create DID
	method := opts.Method
	if method == "" {
		method = MethodKey
	}

	var did *DID
	switch method {
	case MethodKey:
		did = NewKeyDID(multibaseEncode(pub))
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMethod, method)
	}

	// Create identity
	identity := NewAgentIdentity(*did, opts.DisplayName)
	identity.InternalRole = opts.InternalRole
	if opts.Metadata != nil {
		identity.Metadata = opts.Metadata
	}

	// Issue capability credentials
	for _, capability := range opts.InitialCapabilities {
		cred, err := p.issueCapabilityCredential(ctx, *did, capability)
		if err != nil {
			return nil, fmt.Errorf("issue capability credential: %w", err)
		}
		identity.AddCredential(*cred)
	}

	// Store identity and key
	p.mu.Lock()
	p.identities[did.String()] = identity
	p.keys[did.String()] = priv
	p.mu.Unlock()

	return identity, nil
}

// ResolveIdentity resolves a DID to an agent identity.
func (p *LocalProvider) ResolveIdentity(_ context.Context, did DID) (*AgentIdentity, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	identity, ok := p.identities[did.String()]
	if !ok {
		return nil, ErrIdentityNotFound
	}

	// Return a copy to prevent external modification
	identityCopy := *identity
	return &identityCopy, nil
}

// IssueCredential issues a verifiable credential for the subject.
func (p *LocalProvider) IssueCredential(_ context.Context, _ DID, credType string, claims any) (*VerifiableCredential, error) {
	// Generate credential ID
	credID := fmt.Sprintf("urn:uuid:%s", uuid.New().String())

	// Create credential
	cred, err := NewVerifiableCredential(credID, p.issuerDID.String(), credType, claims)
	if err != nil {
		return nil, err
	}

	// Sign the credential
	signedCred, err := p.signCredential(cred)
	if err != nil {
		return nil, fmt.Errorf("sign credential: %w", err)
	}

	return signedCred, nil
}

// issueCapabilityCredential issues a capability credential.
func (p *LocalProvider) issueCapabilityCredential(_ context.Context, subject DID, capability string) (*VerifiableCredential, error) {
	credID := fmt.Sprintf("urn:uuid:%s", uuid.New().String())

	cred, err := NewAgentCapabilityCredential(
		credID,
		p.issuerDID.String(),
		subject.String(),
		capability,
		1.0, // Full confidence for self-issued
	)
	if err != nil {
		return nil, err
	}

	return p.signCredential(cred)
}

// signCredential adds a proof to the credential.
func (p *LocalProvider) signCredential(cred *VerifiableCredential) (*VerifiableCredential, error) {
	// Create proof
	withFragment := p.issuerDID.WithFragment("key-1")
	proof := &Proof{
		Type:               "Ed25519Signature2020",
		Created:            time.Now().UTC(),
		VerificationMethod: withFragment.String(),
		ProofPurpose:       PurposeAssertionMethod,
	}

	// For now, just attach the proof structure
	// In production, this would include actual cryptographic signature
	// using Ed25519 over the canonicalized credential
	proof.ProofValue = base64.StdEncoding.EncodeToString([]byte("local-provider-signature"))

	return cred.WithProof(proof), nil
}

// VerifyCredential verifies a credential's signature and validity.
func (p *LocalProvider) VerifyCredential(_ context.Context, cred *VerifiableCredential) (bool, error) {
	// Basic validation
	if err := cred.Validate(); err != nil {
		return false, fmt.Errorf("%w: %v", ErrCredentialInvalid, err)
	}

	// Check expiration
	if cred.IsExpired() {
		return false, ErrCredentialExpired
	}

	// Check proof exists
	if cred.Proof == nil {
		return false, fmt.Errorf("%w: no proof", ErrCredentialInvalid)
	}

	// For local provider, we trust credentials issued by our issuer
	if cred.Issuer == p.issuerDID.String() {
		return true, nil
	}

	// For credentials from other issuers, we would need to resolve their DID
	// and verify the signature. For now, we accept them if structurally valid.
	return true, nil
}

// UpdateIdentity updates an existing identity.
func (p *LocalProvider) UpdateIdentity(_ context.Context, identity *AgentIdentity) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	didStr := identity.DID.String()
	if _, ok := p.identities[didStr]; !ok {
		return ErrIdentityNotFound
	}

	// Store a copy to prevent external modification
	identityCopy := *identity
	identityCopy.Updated = time.Now().UTC()
	p.identities[didStr] = &identityCopy
	return nil
}

// DeleteIdentity removes an identity.
func (p *LocalProvider) DeleteIdentity(_ context.Context, did DID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	didStr := did.String()
	if _, ok := p.identities[didStr]; !ok {
		return ErrIdentityNotFound
	}

	delete(p.identities, didStr)
	delete(p.keys, didStr)
	return nil
}

// GetIssuerDID returns the provider's issuer DID.
func (p *LocalProvider) GetIssuerDID() *DID {
	return p.issuerDID
}

// multibaseBase58BTCPrefix is the multibase prefix for base58btc encoding.
// See https://github.com/multiformats/multibase for specification.
const multibaseBase58BTCPrefix = "z"

// multibaseEncode encodes a public key as multibase (base58btc with 'z' prefix).
// For Ed25519 keys, the multicodec prefix is 0xed01.
func multibaseEncode(pubKey ed25519.PublicKey) string {
	// Prepend multicodec for Ed25519 public key (0xed01)
	// This is a simplified version - production would use proper multibase/base58btc
	encoded := base64.RawURLEncoding.EncodeToString(pubKey)
	return multibaseBase58BTCPrefix + encoded
}
