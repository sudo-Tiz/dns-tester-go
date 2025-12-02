// Package models defines API request/response structures.
package models

import (
	"fmt"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/normalize"
)

const (
	// MaxDNSServersPerReq limits servers per request to prevent resource exhaustion.
	MaxDNSServersPerReq = 50
)

// DNSServer represents a DNS server target with optional tags
// @Description DNS server configuration with protocol://host:port format
type DNSServer struct {
	Target string   `json:"target" example:"udp://8.8.8.8:53"`              // DNS server in format protocol://host:port
	Tags   []string `json:"tags,omitempty" example:"GOOGLE,PRIMARY,PUBLIC"` // Optional tags for identification
}

// Validate delegates target validation to normalize.Target.
func (d *DNSServer) Validate() error {
	if d.Target == "" {
		return fmt.Errorf("DNS server target cannot be empty")
	}

	if _, err := normalize.Target(d.Target); err != nil {
		return fmt.Errorf("invalid DNS server target '%s': %w", d.Target, err)
	}

	return nil
}

// DNSLookupRequest represents a DNS lookup API request
// @Description DNS lookup request with domain, query type, and optional DNS servers
type DNSLookupRequest struct {
	Domain                string      `json:"domain" binding:"required" example:"example.com"`    // Domain name to query
	DNSServers            []DNSServer `json:"dns_servers,omitempty"`                              // DNS servers to query (optional, uses config if empty)
	QType                 string      `json:"qtype" binding:"required" example:"A"`               // Query type (A, AAAA, MX, TXT, etc.)
	TLSInsecureSkipVerify bool        `json:"tls_insecure_skip_verify,omitempty" example:"false"` // Skip TLS certificate verification (testing only)
}

// Validate checks if domain and qtype are valid.
func (r *DNSLookupRequest) Validate() error {
	normalized, err := normalize.Domain(r.Domain)
	if err != nil {
		return fmt.Errorf("invalid domain: %w", err)
	}
	r.Domain = normalized

	normalizedQType, err := normalize.QType(r.QType)
	if err != nil {
		return fmt.Errorf("invalid query type: %w", err)
	}
	r.QType = normalizedQType

	return nil
}

// TaskResponse is returned when a DNS lookup task is enqueued
// @Description Task submission response with unique task ID
type TaskResponse struct {
	TaskID  string `json:"task_id" example:"abc123def456789"`     // Unique task identifier for polling
	Message string `json:"message" example:"DNS lookup enqueued"` // Status message
}

// DNSAnswer represents a single DNS resource record
// @Description DNS resource record with name, type, TTL, and value
type DNSAnswer struct {
	Name  string `json:"name" example:"example.com."`   // DNS name
	Type  string `json:"type" example:"A"`              // Record type
	TTL   uint32 `json:"ttl" example:"3600"`            // Time to live in seconds
	Value string `json:"value" example:"93.184.216.34"` // Record value
}

// DNSLookupResult contains the outcome of a single DNS server query
// @Description Result from a single DNS server query
type DNSLookupResult struct {
	CommandStatus string      `json:"command_status" example:"success"`             // Command execution status
	TimeMs        float64     `json:"time_ms,omitempty" example:"23.45"`            // Query execution time in milliseconds
	Tags          []string    `json:"tags,omitempty" example:"GOOGLE,PRIMARY"`      // Server tags
	RCode         string      `json:"rcode,omitempty" example:"NOERROR"`            // DNS response code
	Name          string      `json:"name,omitempty" example:"example.com."`        // Queried name
	QType         string      `json:"qtype,omitempty" example:"A"`                  // Query type
	Answers       []DNSAnswer `json:"answers,omitempty"`                            // DNS answers
	Error         string      `json:"error,omitempty" example:"connection timeout"` // Error message if query failed
	DNSProtocol   string      `json:"dns_protocol,omitempty" example:"udp"`         // Protocol used (udp, tcp, tls, https, quic)
}

// DNSLookupResults aggregates results from multiple servers
// @Description Aggregated DNS lookup results from all queried servers
type DNSLookupResults struct {
	Details  map[string]DNSLookupResult `json:"details"`                  // Results per DNS server (keyed by target)
	Duration float64                    `json:"duration" example:"0.125"` // Total query duration in seconds
}

// TaskStatusResponse represents task status and optional result
// @Description Task status response with result when completed
type TaskStatusResponse struct {
	TaskID      string            `json:"task_id" example:"abc123def456789"`        // Task identifier
	Status      string            `json:"task_status" example:"SUCCESS"`            // Task status (PENDING, ACTIVE, SUCCESS, FAILURE)
	Result      *DNSLookupResults `json:"task_result,omitempty"`                    // Query results (populated when status is SUCCESS)
	Error       *string           `json:"error,omitempty" example:"worker timeout"` // Error message (populated when status is FAILURE)
	CreatedAt   time.Time         `json:"created_at,omitempty"`                     // Task creation timestamp
	CompletedAt time.Time         `json:"completed_at,omitempty"`                   // Task completion timestamp
}

// HealthResponse indicates API health status
// @Description Health check response
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`                                    // Health status (ok, degraded)
	Warning string `json:"warning,omitempty" example:"no active workers detected"` // Warning message if degraded
}

// ErrorResponse represents an API error response
// @Description Error response returned for failed requests
type ErrorResponse struct {
	Error string `json:"error" example:"rate limit exceeded"` // Error message
}

// ReverseLookupRequest represents a reverse DNS lookup request
// @Description Reverse DNS lookup request for an IP address
type ReverseLookupRequest struct {
	ReverseIP             string      `json:"reverse_ip" binding:"required" example:"8.8.8.8"`    // IP address to reverse lookup
	DNSServers            []DNSServer `json:"dns_servers,omitempty"`                              // DNS servers to query (optional)
	TLSInsecureSkipVerify bool        `json:"tls_insecure_skip_verify,omitempty" example:"false"` // Skip TLS certificate verification
}
