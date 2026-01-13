// Package http provides an HTTP gateway implementation for bridging REST APIs to NATS services.
//
// # Overview
//
// The http package implements a Gateway component that maps HTTP routes to NATS
// request/reply subjects. It translates HTTP requests into NATS messages and forwards
// responses back to HTTP clients, enabling REST API access to internal NATS services.
//
// Key features:
//   - Route mapping from HTTP paths to NATS subjects
//   - Configurable request timeouts per route
//   - CORS support with configurable origins
//   - Request size limits
//   - Distributed tracing via X-Request-ID headers
//   - Error sanitization (prevents internal details leaking to clients)
//
// # Architecture
//
//	┌────────────────────────────────────────────────────────────────────────┐
//	│                         HTTP Gateway                                   │
//	├────────────────────────────────────────────────────────────────────────┤
//	│  HTTP Mux  →  Route Handler  →  NATS Request  →  Service              │
//	│                                  ↓                  ↓                  │
//	│             ←  JSON Response ←  NATS Reply   ←  Response              │
//	└────────────────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Register the gateway and configure routes:
//
//	err := http.Register(registry)
//
// Or configure in a flow definition:
//
//	{
//	    "type": "gateway",
//	    "name": "http",
//	    "config": {
//	        "routes": [
//	            {
//	                "path": "/api/v1/entities/{id}",
//	                "method": "GET",
//	                "nats_subject": "graph.query.entity",
//	                "timeout": "5s"
//	            },
//	            {
//	                "path": "/api/v1/search",
//	                "method": "POST",
//	                "nats_subject": "graph.query.globalSearch",
//	                "timeout": "30s"
//	            }
//	        ],
//	        "enable_cors": true,
//	        "cors_origins": ["https://example.com"],
//	        "max_request_size": 1048576
//	    }
//	}
//
// Register handlers with an HTTP server:
//
//	gateway.RegisterHTTPHandlers("/api", mux)
//
// # Route Configuration
//
// Each route mapping specifies:
//   - path: HTTP path pattern
//   - method: HTTP method (GET, POST, PUT, DELETE)
//   - nats_subject: Target NATS subject for request/reply
//   - timeout: Request timeout (default: from gateway config)
//
// # Request Processing
//
// Request flow:
//  1. Extract or generate X-Request-ID for distributed tracing
//  2. Validate HTTP method against route configuration
//  3. Apply CORS headers if enabled
//  4. Read and validate request body (size limit enforced)
//  5. Forward to NATS subject with timeout
//  6. Return response with Content-Type: application/json
//
// # Error Handling
//
// Errors are mapped to appropriate HTTP status codes:
//   - Invalid errors → 400 Bad Request
//   - Transient errors → 503 Service Unavailable or 504 Gateway Timeout
//   - Fatal errors → 500 Internal Server Error
//   - Pattern matching → 404 Not Found, 403 Forbidden
//
// Error messages are sanitized to prevent information disclosure:
//   - Internal details logged but not returned to clients
//   - NATS subjects never exposed
//   - Generic messages returned (e.g., "service temporarily unavailable")
//
// # Configuration
//
// Gateway configuration options:
//
//	Routes:          []RouteMapping  # Route definitions
//	EnableCORS:      true            # Enable CORS headers
//	CORSOrigins:     ["*"]           # Allowed origins (use specific domains in production)
//	MaxRequestSize:  1048576         # Max request body size (1MB default)
//
// # Distributed Tracing
//
// The gateway supports distributed tracing via request IDs:
//   - Incoming X-Request-ID header is preserved if present
//   - New request ID generated if not provided (crypto/rand based)
//   - Request ID returned in response X-Request-ID header
//   - Request ID can be propagated to downstream NATS services
//
// # Thread Safety
//
// The Gateway is safe for concurrent use. Metrics use atomic operations,
// and state is protected by RWMutex where needed.
//
// # Metrics
//
// The gateway tracks:
//   - requestsTotal: Total HTTP requests received
//   - requestsSuccess: Successful requests
//   - requestsFailed: Failed requests
//   - bytesReceived: Total bytes in request bodies
//   - bytesSent: Total bytes in responses
//
// DataFlow() returns calculated rates from these metrics.
//
// # See Also
//
// Related packages:
//   - [github.com/c360/semstreams/gateway]: Gateway interface and Config types
//   - [github.com/c360/semstreams/natsclient]: NATS connection management
//   - [github.com/c360/semstreams/component]: Component lifecycle interface
package http
