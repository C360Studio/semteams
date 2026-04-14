package identity

import "errors"

// Identity-related errors
var (
	// ErrDisplayNameRequired indicates display name is required.
	ErrDisplayNameRequired = errors.New("display name is required")

	// ErrUnknownProviderType indicates an unknown provider type.
	ErrUnknownProviderType = errors.New("unknown provider type")

	// ErrIdentityNotFound indicates the identity was not found.
	ErrIdentityNotFound = errors.New("identity not found")

	// ErrCredentialInvalid indicates the credential is invalid.
	ErrCredentialInvalid = errors.New("credential is invalid")

	// ErrCredentialExpired indicates the credential has expired.
	ErrCredentialExpired = errors.New("credential has expired")

	// ErrSignatureInvalid indicates the signature verification failed.
	ErrSignatureInvalid = errors.New("signature verification failed")

	// ErrKeyNotFound indicates the key was not found.
	ErrKeyNotFound = errors.New("key not found")

	// ErrProviderNotConfigured indicates the provider is not configured.
	ErrProviderNotConfigured = errors.New("provider not configured")

	// ErrUnsupportedMethod indicates the DID method is not supported.
	ErrUnsupportedMethod = errors.New("unsupported DID method")
)
