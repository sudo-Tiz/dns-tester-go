// Package app composes API server and task queue client.
// Chooses memory or Asynq backend based on Redis URL presence.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/sudo-tiz/dns-tester-go/internal/api"
	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/tasks"
)

// APIApp wraps server and tasks client for lifecycle management.
type APIApp struct {
	cfg         *config.APIConfig
	tasksClient tasks.ClientInterface
	server      *api.Server
}

// NewAPIApp chooses memory or Asynq client - no Redis means in-memory mode.
func NewAPIApp(cfg *config.APIConfig, redisURL string) (*APIApp, error) {
	a := &APIApp{cfg: cfg}

	var client tasks.ClientInterface
	if redisURL == "" {
		client = tasks.NewMemoryClient(cfg)
	} else {
		redisAddr := redisURL
		if u, err := url.Parse(redisURL); err == nil {
			redisAddr = u.Host
		}
		client = tasks.NewClient(redisAddr, 24*time.Hour)
	}
	a.tasksClient = client

	srv := api.NewServer(cfg)
	if a.tasksClient != nil {
		srv.SetTasksClient(a.tasksClient)
	}
	a.server = srv

	return a, nil
}

// Run starts HTTP server with configured address.
func (a *APIApp) Run(addr string) error {
	if a.server == nil {
		return fmt.Errorf("server not initialized")
	}
	slog.Info("Starting API", "address", addr)
	return a.server.Run(addr)
}

// Shutdown closes task client connections.
func (a *APIApp) Shutdown(_ context.Context) error {
	if a.tasksClient != nil {
		return a.tasksClient.Close()
	}
	return nil
}
