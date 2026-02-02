// Package acme provides ACME client functionality for automated certificate management.
//
// # Overview
//
// The acme package implements an ACME (Automatic Certificate Management Environment)
// client using the lego library. It handles certificate lifecycle management including
// account creation, certificate issuance, renewal, and storage for use with TLS servers.
//
// Key features:
//   - Automatic account registration with ACME servers
//   - HTTP-01 and TLS-ALPN-01 challenge support
//   - Certificate renewal with configurable lead time
//   - Persistent storage of accounts and certificates
//   - Support for private ACME servers (step-ca, etc.)
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                         ACME Client                                 │
//	├─────────────────────────────────────────────────────────────────────┤
//	│  Account Management   │  Certificate Ops    │  Storage              │
//	│  - Registration       │  - Obtain           │  - account.json       │
//	│  - Key storage        │  - Renew            │  - account.key        │
//	│                       │  - RenewalLoop      │  - certificate.pem    │
//	│                       │                     │  - certificate.key    │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                      ACME Server (CA)                               │
//	│  Let's Encrypt / step-ca / ZeroSSL / etc.                           │
//	└─────────────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Create an ACME client and obtain a certificate:
//
//	cfg := acme.Config{
//	    DirectoryURL:  "https://acme-v02.api.letsencrypt.org/directory",
//	    Email:         "admin@example.com",
//	    Domains:       []string{"example.com", "www.example.com"},
//	    ChallengeType: "http-01",
//	    RenewBefore:   24 * time.Hour,
//	    StoragePath:   "/etc/ssl/acme",
//	}
//
//	client, err := acme.NewClient(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Obtain certificate
//	cert, err := client.ObtainCertificate(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Check and renew if needed:
//
//	cert, renewed, err := client.RenewCertificateIfNeeded(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if renewed {
//	    log.Println("Certificate was renewed")
//	}
//
// Background renewal loop:
//
//	err := client.StartRenewalLoop(ctx, 1*time.Hour, func(cert *tls.Certificate) {
//	    // Hot-reload certificate in TLS server
//	    tlsConfig.Certificates = []tls.Certificate{*cert}
//	})
//
// # Configuration
//
// Config fields:
//
//	DirectoryURL   string         // ACME server directory URL (required)
//	Email          string         // Contact email for account (required)
//	Domains        []string       // Domains for certificate (required)
//	ChallengeType  string         // "http-01" or "tls-alpn-01" (default: "http-01")
//	RenewBefore    time.Duration  // Renew this long before expiry (default: 8h)
//	StoragePath    string         // Directory for storing certs/keys (required)
//	CABundle       string         // Optional CA bundle for private ACME servers
//
// # Challenge Types
//
// HTTP-01 Challenge:
//   - Requires port 80 accessible
//   - ACME server makes HTTP request to validate domain
//   - Easier to set up, works through most proxies
//
// TLS-ALPN-01 Challenge:
//   - Requires port 443 accessible
//   - ACME server validates via TLS handshake
//   - Better for environments where only TLS port is exposed
//
// # Storage
//
// The client stores credentials in the configured StoragePath:
//
//	{StoragePath}/account.json   - Account metadata and registration
//	{StoragePath}/account.key    - Account private key (ECDSA P-256)
//	{StoragePath}/certificate.pem - Certificate chain (PEM)
//	{StoragePath}/certificate.key - Certificate private key (PEM)
//
// File permissions are set to 0600 (keys) and 0644 (certificates).
//
// # Private ACME Servers
//
// For private ACME servers like step-ca, specify CABundle:
//
//	cfg := acme.Config{
//	    DirectoryURL: "https://ca.internal:9000/acme/acme/directory",
//	    CABundle:     "/etc/ssl/step-ca-root.pem",
//	    // ...
//	}
//
// The CABundle is used to validate the ACME server's TLS certificate.
//
// # Thread Safety
//
// The Client is safe for concurrent use. Certificate operations are
// serialized internally.
//
// # Error Handling
//
// Errors are classified using the errs package:
//   - Invalid config: errs.WrapInvalid (bad URL, missing required fields)
//   - Fatal errors: errs.WrapFatal (file system errors, key generation)
//   - Transient errors: errs.WrapTransient (network issues, ACME server errors)
//
// Transient errors during renewal are logged but don't crash the service.
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semstreams/pkg/tlsutil]: TLS configuration with ACME integration
//   - [github.com/c360studio/semstreams/pkg/security]: Security configuration types
//   - [github.com/go-acme/lego/v4]: Underlying ACME library
package acme
