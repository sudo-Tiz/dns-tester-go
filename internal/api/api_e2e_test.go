//go:build e2e

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
)

const (
	defaultAPIURL     = "http://localhost:5000"
	testDomain        = "example.com"
	testQueryType     = "A"
	maxPollTime       = 30 * time.Second
	pollInterval      = 2 * time.Second
	rateLimitMaxTries = 150
)

// getAPIBaseURL returns the API URL for testing
func getAPIBaseURL() string {
	if url := os.Getenv("API_BASE_URL"); url != "" {
		return url
	}
	return defaultAPIURL
}

// Test01_DNSLookupAllServers tests DNS lookup against all servers from config.example.yaml
func Test01_DNSLookupAllServers(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "1" {
		t.Skip("E2E tests skipped (set RUN_E2E_TESTS=1 to run)")
	}

	// Load config.example.yaml
	cfg, err := config.LoadConfig("../../conf/config.example.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Servers) == 0 {
		t.Fatal("No DNS servers found in config.example.yaml")
	}

	apiURL := getAPIBaseURL()
	t.Logf("Testing against API: %s", apiURL)

	// Build list of all DNS servers from config
	var dnsServers []map[string]interface{}
	for _, server := range cfg.Servers {
		for _, service := range server.Services {
			target := buildDNSTarget(server, string(service))
			if target == "" {
				continue
			}
			dnsServers = append(dnsServers, map[string]interface{}{
				"target": target,
				"tags":   server.Tags,
			})
		}
	}

	t.Logf("Testing %d DNS server configurations", len(dnsServers))

	// Single request with all servers
	payload := map[string]interface{}{
		"domain":                   testDomain,
		"qtype":                    testQueryType,
		"dns_servers":              dnsServers,
		"tls_insecure_skip_verify": false,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	// Submit DNS lookup request
	resp, err := http.Post(apiURL+"/dns-lookup", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to submit DNS lookup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200/202, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var lookupResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&lookupResp); err != nil {
		t.Fatalf("Failed to decode lookup response: %v", err)
	}

	taskID, ok := lookupResp["task_id"].(string)
	if !ok || taskID == "" {
		t.Fatalf("No task_id returned: %v", lookupResp)
	}

	t.Logf("Task ID: %s", taskID)

	// Poll for result
	result := pollForTaskResult(t, apiURL, taskID)

	// Verify result
	if result.Status != "SUCCESS" {
		errorMsg := ""
		if result.Error != nil {
			errorMsg = *result.Error
		}
		t.Errorf("Task did not complete successfully: status=%s, error=%s", result.Status, errorMsg)
		return
	}

	if result.Result == nil {
		t.Fatal("Task completed but result is nil")
	}

	if len(result.Result.Details) == 0 {
		t.Fatal("Task completed but no details in result")
	}

	// Verify all targets were queried
	successCount := 0
	for _, serverMap := range dnsServers {
		target := serverMap["target"].(string)
		detail, exists := result.Result.Details[target]
		if !exists {
			t.Errorf("Target %s not found in results", target)
			continue
		}

		if detail.CommandStatus == "ok" && detail.RCode == "NOERROR" {
			successCount++
			t.Logf("✅ %s: %d answers", target, len(detail.Answers))
		} else {
			t.Logf("⚠️  %s: status=%s rcode=%s", target, detail.CommandStatus, detail.RCode)
		}
	}

	t.Logf("Successfully queried %d/%d DNS servers", successCount, len(dnsServers))

	// Sleep before next test
	t.Log("Sleeping 2s before next test...")
	time.Sleep(2 * time.Second)
}

// buildDNSTarget constructs a DNS target URL from server config and service type
func buildDNSTarget(server config.DNSServer, service string) string {
	port := server.Port
	if port == 0 {
		port = 53 // Default port
	}

	switch service {
	case "do53/udp":
		if server.IP != "" {
			return fmt.Sprintf("udp://%s:%d", server.IP, port)
		}
		return fmt.Sprintf("udp://%s:%d", server.Hostname, port)
	case "do53/tcp":
		if server.IP != "" {
			return fmt.Sprintf("tcp://%s:%d", server.IP, port)
		}
		return fmt.Sprintf("tcp://%s:%d", server.Hostname, port)
	case "dot":
		return fmt.Sprintf("tls://%s:853", server.Hostname)
	case "doh":
		return fmt.Sprintf("https://%s/dns-query", server.Hostname)
	case "doq":
		return fmt.Sprintf("quic://%s:853", server.Hostname)
	default:
		return ""
	}
}

// pollForTaskResult polls the API for task completion
func pollForTaskResult(t *testing.T, apiURL, taskID string) models.TaskStatusResponse {
	t.Helper()

	deadline := time.Now().Add(maxPollTime)
	var lastResult models.TaskStatusResponse

	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", apiURL, taskID))
		if err != nil {
			t.Logf("Poll error: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(bodyBytes, &lastResult); err != nil {
			t.Logf("Parse error: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		t.Logf("Task status: %s", lastResult.Status)

		// Check if task is completed
		if lastResult.Status == "SUCCESS" || lastResult.Status == "FAILED" || lastResult.Status == "FAILURE" {
			return lastResult
		}

		time.Sleep(pollInterval)
	}

	t.Fatalf("Timeout waiting for task result after %v. Last status: %s", maxPollTime, lastResult.Status)
	return lastResult
}

// Test02_MetricsEndpoint tests that Prometheus metrics are exposed correctly
// Must run AFTER Test01 to see metrics from DNS lookups
func Test02_MetricsEndpoint(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "1" {
		t.Skip("E2E tests skipped (set RUN_E2E_TESTS=1 to run)")
	}

	apiURL := getAPIBaseURL()
	t.Logf("Testing API metrics endpoint: %s/metrics", apiURL)

	resp, err := http.Get(apiURL + "/metrics")
	if err != nil {
		t.Fatalf("API metrics endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read API metrics: %v", err)
	}

	metricsText := string(body)

	// Check for general Prometheus metrics (always present)
	if !strings.Contains(metricsText, "go_goroutines") {
		t.Fatalf("Basic Prometheus metrics not found (go_goroutines)")
	}

	t.Log("✅ API Prometheus metrics endpoint working")

	// Check for DNS-specific metrics
	// In dev mode (memory client): resolver runs in-process, metrics should be here
	// In prod mode (redis workers): metrics are on worker endpoints (:9091)
	dnsMetrics := []string{
		"dns_lookup_total",
		"dns_lookup_duration_seconds",
	}

	foundDNSMetrics := 0
	for _, metric := range dnsMetrics {
		if strings.Contains(metricsText, metric) {
			foundDNSMetrics++
			t.Logf("✅ Found metric: %s", metric)
		}
	}

	if foundDNSMetrics == len(dnsMetrics) {
		t.Logf("✅ All %d DNS metrics found (dev mode: resolver in-process)", foundDNSMetrics)
	} else if foundDNSMetrics > 0 {
		t.Logf("⚠️  Found %d/%d DNS metrics", foundDNSMetrics, len(dnsMetrics))
	} else {
		t.Logf("ℹ️  No DNS metrics found (expected in prod mode with separate workers)")
		t.Log("ℹ️  Worker metrics are exposed on :9091/metrics in production")
	}

	// Sleep before next test
	t.Log("Sleeping 1s before next test...")
	time.Sleep(1 * time.Second)
}

// Test03_RateLimiting tests that rate limiting works correctly
// Must run LAST as it exhausts the rate limit
func Test03_RateLimiting(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "1" {
		t.Skip("E2E tests skipped (set RUN_E2E_TESTS=1 to run)")
	}

	apiURL := getAPIBaseURL()
	t.Logf("Testing rate limiting against: %s", apiURL)

	payload := map[string]interface{}{
		"domain": "ratelimit-test.example",
		"qtype":  testQueryType,
		"dns_servers": []map[string]interface{}{
			{
				"target": "udp://8.8.8.8:53",
				"tags":   []string{"ratelimit-test"},
			},
		},
		"tls_insecure_skip_verify": false,
	}

	jsonData, _ := json.Marshal(payload)

	var got429 bool
	var successCount int

	for i := 0; i < rateLimitMaxTries; i++ {
		resp, err := http.Post(apiURL+"/dns-lookup", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Logf("Request %d failed with error: %v", i+1, err)
			continue
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		if statusCode == http.StatusOK || statusCode == http.StatusAccepted {
			successCount++
		} else if statusCode == http.StatusTooManyRequests {
			t.Logf("✅ Rate limit triggered at request %d (after %d successful requests)", i+1, successCount)
			got429 = true
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !got429 {
		t.Errorf("❌ Rate limit not triggered after %d requests (%d succeeded) - rate limiting may be disabled or threshold too high",
			rateLimitMaxTries, successCount)
	}
}
