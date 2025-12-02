// Package tasks provides async DNS lookup queue using Asynq/Redis or in-memory fallback.
// Delegates task queue to Asynq, caches results in Redis for 24h TTL.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
)

const (
	// TaskTypeDNSLookup is the task type identifier for DNS lookup tasks
	TaskTypeDNSLookup = "dns:lookup"
)

// Client wraps Asynq for task enqueueing and Redis for result caching.
type Client struct {
	asynqClient *asynq.Client
	inspector   *asynq.Inspector
	redisClient *redis.Client
	resultTTL   time.Duration
}

// ClientInterface allows swapping between Asynq and memory implementations.
type ClientInterface interface {
	EnqueueDNSLookup(ctx context.Context, domain, qtype string, servers []models.DNSServer, tlsInsecure bool) (string, error)
	GetTaskStatus(ctx context.Context, taskID string) (*models.TaskStatusResponse, error)
	Close() error
}

// NewClient creates Asynq client with Redis result backend.
func NewClient(redisAddr string, resultTTL time.Duration) *Client {
	redisOpts := asynq.RedisClientOpt{Addr: redisAddr}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	return &Client{
		asynqClient: asynq.NewClient(redisOpts),
		inspector:   asynq.NewInspector(redisOpts),
		redisClient: rdb,
		resultTTL:   resultTTL,
	}
}

// EnqueueDNSLookup creates task with UUID, enqueues to Asynq with 3 retry max.
func (c *Client) EnqueueDNSLookup(ctx context.Context, domain, qtype string, servers []models.DNSServer, tlsInsecure bool) (string, error) {
	id := uuid.NewString()

	payload := map[string]interface{}{
		"task_id":      id,
		"domain":       domain,
		"qtype":        qtype,
		"servers":      servers,
		"tls_insecure": tlsInsecure,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	task := asynq.NewTask(TaskTypeDNSLookup, data)
	opts := []asynq.Option{
		asynq.TaskID(id),
		asynq.MaxRetry(3),
	}

	if _, err := c.asynqClient.EnqueueContext(ctx, task, opts...); err != nil {
		return "", fmt.Errorf("enqueue failed: %w", err)
	}

	return id, nil
}

// Close shuts down all connections.
func (c *Client) Close() error {
	var errs []error

	if err := c.inspector.Close(); err != nil {
		errs = append(errs, fmt.Errorf("inspector: %w", err))
	}

	if err := c.redisClient.Close(); err != nil {
		errs = append(errs, fmt.Errorf("redis: %w", err))
	}

	if err := c.asynqClient.Close(); err != nil {
		errs = append(errs, fmt.Errorf("asynq: %w", err))
	}

	return errors.Join(errs...)
}

// HasActiveWorkers checks Asynq inspector for connected workers.
func (c *Client) HasActiveWorkers(_ context.Context) bool {
	servers, err := c.inspector.Servers()
	if err != nil {
		return false
	}

	return len(servers) > 0
}

// GetTaskStatus checks Redis cache first, falls back to Asynq inspector.
// Pragmatic approach: cache completed results, poll Asynq for pending/active.
func (c *Client) GetTaskStatus(ctx context.Context, taskID string) (*models.TaskStatusResponse, error) {
	resultKey := fmt.Sprintf("dnstester:result:%s", taskID)
	resultJSON, err := c.redisClient.Get(ctx, resultKey).Result()

	if err == nil {
		// Result cached - task completed
		var res models.DNSLookupResults
		if json.Unmarshal([]byte(resultJSON), &res) == nil {
			return &models.TaskStatusResponse{
				TaskID: taskID,
				Status: "SUCCESS",
				Result: &res,
			}, nil
		}
	}

	// Not cached - check Asynq queue
	taskInfo, err := c.inspector.GetTaskInfo("default", taskID)
	if err != nil {
		return nil, fmt.Errorf("not found")
	}

	response := &models.TaskStatusResponse{
		TaskID:      taskID,
		CreatedAt:   taskInfo.NextProcessAt,
		CompletedAt: taskInfo.CompletedAt,
	}

	switch taskInfo.State {
	case asynq.TaskStateActive:
		response.Status = "ACTIVE"
	case asynq.TaskStateRetry:
		response.Status = "RETRY"
	case asynq.TaskStateArchived:
		response.Status = "FAILURE"
		if taskInfo.LastErr != "" {
			errMsg := taskInfo.LastErr
			response.Error = &errMsg
		}
	default:
		response.Status = "PENDING"
	}

	return response, nil
}
