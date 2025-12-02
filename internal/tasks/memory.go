package tasks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
	"github.com/sudo-tiz/dns-tester-go/internal/resolver"
)

type memoryClient struct {
	mu                   sync.Mutex
	tasks                map[string]*models.DNSLookupResults
	ttl                  map[string]time.Time
	timeout              time.Duration
	maxConcurrentQueries int
	maxRetries           int
}

// NewMemoryClient creates in-memory task queue for dev/testing without Redis.
// Uses background context for queries to avoid HTTP timeout coupling.
// Returns ClientInterface for consistent API with Asynq implementation.
func NewMemoryClient(cfg *config.APIConfig) ClientInterface {
	timeout := time.Duration(cfg.GetDNSTimeout()) * time.Second
	return &memoryClient{
		tasks:                make(map[string]*models.DNSLookupResults),
		ttl:                  make(map[string]time.Time),
		timeout:              timeout,
		maxConcurrentQueries: cfg.GetMaxConcurrentQueries(),
		maxRetries:           cfg.GetMaxRetries(),
	}
}

// EnqueueDNSLookup executes DNS query in background goroutine.
// Pragmatic choice: decouple from HTTP request context to avoid premature cancellation.
func (m *memoryClient) EnqueueDNSLookup(_ context.Context, domain, qtype string, servers []models.DNSServer, tlsInsecure bool) (string, error) {
	id := "mem-" + time.Now().Format("20060102150405.000000000")

	m.mu.Lock()
	m.tasks[id] = nil
	m.ttl[id] = time.Now().Add(1 * time.Hour)
	m.mu.Unlock()

	// Use independent context - HTTP request may timeout before query completes
	go func() {
		taskCtx := context.Background()
		start := time.Now()
		results := make(map[string]models.DNSLookupResult)
		if len(servers) > 0 {
			results = resolver.RunQueries(taskCtx, domain, qtype, servers, tlsInsecure, m.timeout, m.maxConcurrentQueries, m.maxRetries)
		}
		duration := time.Since(start).Seconds()

		lookupResults := &models.DNSLookupResults{
			Details:  results,
			Duration: duration,
		}

		m.mu.Lock()
		m.tasks[id] = lookupResults
		m.mu.Unlock()
	}()

	return id, nil
}

func (m *memoryClient) Close() error {
	return nil
}

// GetTaskStatus returns PENDING while executing, SUCCESS when done.
func (m *memoryClient) GetTaskStatus(_ context.Context, taskID string) (*models.TaskStatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.ttl[taskID]
	if !exists {
		return nil, fmt.Errorf("not found")
	}

	res := m.tasks[taskID]

	if res == nil {
		return &models.TaskStatusResponse{
			TaskID: taskID,
			Status: "PENDING",
		}, nil
	}

	return &models.TaskStatusResponse{
		TaskID: taskID,
		Status: "SUCCESS",
		Result: res,
	}, nil
}
