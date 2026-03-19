package metric

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/security"
	"github.com/c360studio/semstreams/pkg/tlsutil"
)

// Server represents the metrics HTTP server
type Server struct {
	port     int
	path     string
	server   *http.Server
	registry *MetricsRegistry
	security security.Config
	mu       sync.Mutex // protects server field
}

// NewServer creates a new metrics server with the provided registry
func NewServer(port int, path string, registry *MetricsRegistry, securityCfg security.Config) *Server {
	if path == "" {
		path = "/metrics"
	}
	if port == 0 {
		port = 9090
	}

	return &Server{
		port:     port,
		path:     path,
		registry: registry,
		security: securityCfg,
	}
}

// Start starts the metrics HTTP server. Blocks until the server is stopped.
func (s *Server) Start() error {
	// Hold the lock only for setup, NOT during ListenAndServe.
	// Stop() needs to acquire this lock to call server.Close().
	s.mu.Lock()

	// Check if server is already running
	if s.server != nil {
		s.mu.Unlock()
		return errs.WrapInvalid(
			fmt.Errorf("server already running"),
			"Server", "Start", "cannot start server that is already running")
	}

	// Validate that we have a registry
	if s.registry == nil {
		s.mu.Unlock()
		return errs.WrapFatal(
			fmt.Errorf("nil registry"),
			"Server", "Start", "metrics registry not provided")
	}

	mux := http.NewServeMux()

	// Create Prometheus HTTP handler
	handler := promhttp.HandlerFor(
		s.registry.PrometheusRegistry(),
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)

	// Register the handler
	mux.Handle(s.path, handler)

	// Add a health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Add a root handler with information
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html>
<head><title>SemStreams Metrics</title></head>
<body>
<h1>SemStreams Metrics Server</h1>
<p><a href="%s">Metrics</a></p>
<p><a href="/health">Health</a></p>
</body>
</html>`, s.path)
	})

	// Create the server
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Configure TLS if enabled at platform level
	if s.security.TLS.Server.Enabled {
		tlsConfig, err := tlsutil.LoadServerTLSConfig(s.security.TLS.Server)
		if err != nil {
			s.mu.Unlock()
			return errs.WrapFatal(err, "Server", "Start", "load TLS config")
		}
		s.server.TLSConfig = tlsConfig
	}

	// Release lock BEFORE blocking on ListenAndServe — Stop() needs the lock
	// to call server.Close() which unblocks ListenAndServe.
	s.mu.Unlock()

	// Start HTTP or HTTPS server (blocks until Close/Shutdown is called)
	var err error
	if s.security.TLS.Server.Enabled {
		err = s.server.ListenAndServeTLS("", "")
	} else {
		err = s.server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		return errs.WrapFatal(err, "Server", "Start",
			fmt.Sprintf("failed to start server on port %d", s.port))
	}

	return nil
}

// Stop stops the metrics server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		err := s.server.Close()
		s.server = nil // reset server field to allow restart
		if err != nil {
			return errs.WrapTransient(err, "Server", "Stop",
				"failed to stop HTTP server")
		}
	}
	return nil
}

// Address returns the server address
func (s *Server) Address() string {
	scheme := "http"
	if s.security.TLS.Server.Enabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://localhost:%d%s", scheme, s.port, s.path)
}
