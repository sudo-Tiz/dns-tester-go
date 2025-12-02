// Package api provides HTTP client for DNS Tester API.
package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/models"
)

// Client wraps http.Client for API requests.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient configures HTTP client with optional TLS verification skip.
func NewClient(baseURL string, timeout time.Duration, insecure bool) *Client {
	tr := &http.Transport{}
	if insecure {
		//nolint:gosec
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: timeout, Transport: tr},
	}
}

// EnqueueDNSLookup posts DNS lookup request to API.
func (c *Client) EnqueueDNSLookup(ctx context.Context, req models.DNSLookupRequest) (string, error) {
	return c.postTask(ctx, "/dns-lookup", req)
}

// EnqueueReverseLookup wraps reverse IP to PTR lookup for Python dnstester compat.
func (c *Client) EnqueueReverseLookup(ctx context.Context, reverseIP string, servers []models.DNSServer, skipVerify bool) (string, error) {
	req := struct {
		ReverseIP             string             `json:"reverse_ip"`
		DNSServers            []models.DNSServer `json:"dns_servers,omitempty"`
		TLSInsecureSkipVerify bool               `json:"tls_insecure_skip_verify,omitempty"`
	}{
		ReverseIP:             reverseIP,
		DNSServers:            servers,
		TLSInsecureSkipVerify: skipVerify,
	}
	return c.postTask(ctx, "/reverse-lookup", req)
}

// postTask marshals payload, POSTs to path, returns task ID.
func (c *Client) postTask(ctx context.Context, path string, payload interface{}) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := c.baseURL + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("api error: %s", string(body))
	}
	var out models.TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.TaskID, nil
}

// GetTaskStatus polls task status from API.
func (c *Client) GetTaskStatus(ctx context.Context, taskID string) (*models.TaskStatusResponse, error) {
	url := c.baseURL + "/tasks/" + taskID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error: %s", string(body))
	}
	var out models.TaskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
