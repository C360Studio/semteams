// Package tlsutil provides TLS configuration utilities for secure connections.
//
// # Overview
//
// The tlsutil package bridges security configuration types (pkg/security) and
// Go's crypto/tls package. It creates properly configured tls.Config values
// for both server and client use cases, with support for mTLS and ACME automation.
//
// Key features:
//   - Server TLS configuration (manual or ACME-managed)
//   - Client TLS configuration (with system CA integration)
//   - Mutual TLS (mTLS) for client certificate validation/provision
//   - ACME integration with automatic renewal and hot-reload
//   - Graceful fallback when ACME is unavailable
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                     security.Config                                 │
//	│  (Platform-wide security configuration)                             │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        tlsutil                                      │
//	│  LoadServerTLSConfig()  │  LoadClientTLSConfig()                    │
//	│  LoadServerTLSConfigWithMTLS()  │  LoadClientTLSConfigWithMTLS()    │
//	│  LoadServerTLSConfigWithACME()  │  LoadClientTLSConfigWithACME()    │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        *tls.Config                                  │
//	│  (Ready to use with http.Server, tls.Dial, etc.)                    │
//	└─────────────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Basic server TLS:
//
//	cfg := security.ServerTLSConfig{
//	    Enabled:  true,
//	    CertFile: "/etc/ssl/server.crt",
//	    KeyFile:  "/etc/ssl/server.key",
//	}
//
//	tlsConfig, err := tlsutil.LoadServerTLSConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	server := &http.Server{
//	    Addr:      ":443",
//	    TLSConfig: tlsConfig,
//	}
//
// Server with mTLS (require client certificates):
//
//	tlsConfig, err := tlsutil.LoadServerTLSConfigWithMTLS(
//	    cfg,
//	    security.ServerMTLSConfig{
//	        Enabled:           true,
//	        ClientCAFiles:     []string{"/etc/ssl/client-ca.pem"},
//	        RequireClientCert: true,
//	        AllowedClientCNs:  []string{"service-a"},
//	    },
//	)
//
// Client TLS (uses system CAs plus additional):
//
//	cfg := security.ClientTLSConfig{
//	    CAFiles: []string{"/etc/ssl/internal-ca.pem"},
//	}
//
//	tlsConfig, err := tlsutil.LoadClientTLSConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	client := &http.Client{
//	    Transport: &http.Transport{TLSClientConfig: tlsConfig},
//	}
//
// Client with mTLS (present client certificate):
//
//	tlsConfig, err := tlsutil.LoadClientTLSConfigWithMTLS(
//	    cfg,
//	    security.ClientMTLSConfig{
//	        Enabled:  true,
//	        CertFile: "/etc/ssl/client.crt",
//	        KeyFile:  "/etc/ssl/client.key",
//	    },
//	)
//
// # ACME Integration
//
// Server with automatic certificate management:
//
//	cfg := security.ServerTLSConfig{
//	    Enabled: true,
//	    Mode:    "acme",
//	    ACME: security.ACMEConfig{
//	        Enabled:       true,
//	        DirectoryURL:  "https://acme-v02.api.letsencrypt.org/directory",
//	        Email:         "admin@example.com",
//	        Domains:       []string{"example.com"},
//	        ChallengeType: "http-01",
//	        StoragePath:   "/etc/ssl/acme",
//	    },
//	}
//
//	tlsConfig, cleanup, err := tlsutil.LoadServerTLSConfigWithACME(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()  // Stops background renewal
//
//	server := &http.Server{
//	    Addr:      ":443",
//	    TLSConfig: tlsConfig,
//	}
//
// The ACME functions:
//   - Obtain certificates at startup
//   - Renew before expiry (background goroutine)
//   - Hot-reload new certificates (no restart needed)
//   - Fall back to manual certs if ACME fails
//
// # Client Certificate Verification
//
// For mTLS servers, optional CN whitelist:
//
//	mtlsCfg := security.ServerMTLSConfig{
//	    Enabled:          true,
//	    ClientCAFiles:    []string{"/etc/ssl/client-ca.pem"},
//	    AllowedClientCNs: []string{"service-a", "service-b"},
//	}
//
// Only certificates with matching Common Names are accepted.
//
// # TLS Version Configuration
//
// Supported MinVersion values:
//   - "1.2" - TLS 1.2 (default, widely compatible)
//   - "1.3" - TLS 1.3 (more secure, modern clients only)
//
// If unspecified or invalid, defaults to TLS 1.2.
//
// # Error Handling
//
// Errors are classified using the errs package:
//   - Fatal errors: File not found, invalid PEM data
//   - Transient errors: ACME server unavailable
//
// ACME functions provide automatic fallback to manual certificates
// when ACME is unavailable but manual certs are configured.
//
// # Thread Safety
//
// All Load* functions are safe for concurrent use.
// The returned tls.Config is thread-safe for concurrent reads.
// ACME hot-reload updates are atomic (certificate swap).
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semstreams/pkg/security]: Configuration types
//   - [github.com/c360studio/semstreams/pkg/acme]: ACME client implementation
//   - [crypto/tls]: Go standard library TLS package
package tlsutil
