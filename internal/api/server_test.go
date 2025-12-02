package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
)

const mockTaskID = "mock-task-id"

type mockTasksClient struct{}

func (m *mockTasksClient) Close() error { return nil }
func (m *mockTasksClient) EnqueueDNSLookup(_ context.Context, _ string, _ string, _ []models.DNSServer, _ bool) (string, error) {
	return mockTaskID, nil
}
func (m *mockTasksClient) GetTaskStatus(_ context.Context, id string) (*models.TaskStatusResponse, error) {
	if id != mockTaskID {
		return nil, fmt.Errorf("not found")
	}
	return &models.TaskStatusResponse{TaskID: id, Status: "SUCCESS"}, nil
}

func setupTestServer() *Server {
	cfg := &config.APIConfig{}
	s := NewServer(cfg)
	s.SetTasksClient(&mockTasksClient{})
	return s
}

func TestDNSLookupEndpoint(t *testing.T) {
	server := setupTestServer()

	payload := models.DNSLookupRequest{
		Domain: "github.com",
		QType:  "A",
		DNSServers: []models.DNSServer{
			{Target: "udp://9.9.9.9:53"},
		},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/dns-lookup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response models.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.TaskID == "" {
		t.Error("Expected task_id in response")
	}
}

func TestReverseLookupEndpoint(t *testing.T) {
	server := setupTestServer()

	payload := struct {
		ReverseIP  string             `json:"reverse_ip"`
		DNSServers []models.DNSServer `json:"dns_servers"`
	}{
		ReverseIP: "9.9.9.9",
		DNSServers: []models.DNSServer{
			{Target: "udp://9.9.9.9:53"},
		},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/reverse-lookup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestGetTaskStatusEndpoint(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+mockTaskID, nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response models.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.TaskID != mockTaskID {
		t.Errorf("Expected task_id '%s', got '%s'", mockTaskID, response.TaskID)
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response models.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response.Status)
	}
}
