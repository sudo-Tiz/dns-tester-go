package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/models"
)

func TestGetDNSProtocolFromTarget(t *testing.T) {
	tests := []struct {
		target   string
		expected string
	}{
		{"udp://9.9.9.9:53", "Do53"},
		{"tcp://94.140.14.14:53", "Do53"},
		{"tls://dns.quad9.net:853", "DoT"},
		{"https://dns.quad9.net:443", "DoH"},
		{"quic://dns.adguard.com", "DoQ"},
		{"invalid", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			result := GetDNSProtocolFromTarget(tt.target)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRunQueries(t *testing.T) {
	ctx := context.Background()
	servers := []models.DNSServer{
		{Target: "udp://9.9.9.9:53", Tags: []string{"quad9"}},
		{Target: "udp://94.140.14.14:53", Tags: []string{"adguard"}},
	}

	results := RunQueries(ctx, "github.com", "A", servers, false, DefaultTimeout, 500, 3)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestQueryServer_InvalidTarget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := models.DNSServer{
		Target: "invalid-target",
	}

	_, result := QueryServer(ctx, "github.com", "A", server, false, 1, DefaultTimeout)

	if result.CommandStatus != "error" {
		t.Errorf("Expected error status, got %s", result.CommandStatus)
	}
}
