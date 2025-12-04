package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
	"github.com/sudo-tiz/dns-tester-go/internal/resolver"
	"github.com/sudo-tiz/dns-tester-go/internal/tasks"
)

// NewWorkerCommand creates the 'worker' subcommand for running standalone Redis workers
func NewWorkerCommand() *cobra.Command {
	var configPath string
	var redisURL string
	var concurrency int
	var metricsPort int
	var enableMetrics bool

	// DNS config flags
	var dnsTimeout int
	var maxConcurrentQueries int
	var maxRetries int

	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a standalone DNS Tester worker",
		Long:  `Start a standalone DNS Tester worker that processes tasks from Redis queue. Requires Redis to be configured.`,
		Example: `  # Start worker with default settings
  dnstestergo worker --redis redis://localhost:6379/0

  # Start worker with custom concurrency (number of parallel task processors)
  dnstestergo worker --redis redis://localhost:6379/0 --concurrency 8

  # Start worker with metrics enabled (useful for single worker or dev)
  dnstestergo worker --config /path/to/config.yaml --redis redis://localhost:6379/0 --enable-metrics

  # Override DNS settings
  dnstestergo worker --redis redis://localhost:6379/0 --dns-timeout 10 --max-retries 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorker(cmd, configPath, redisURL, concurrency, metricsPort, enableMetrics,
				dnsTimeout, maxConcurrentQueries, maxRetries)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", os.Getenv("CONFIG_PATH"), "Path to config file")
	cmd.Flags().StringVarP(&redisURL, "redis", "r", os.Getenv("REDIS_URL"), "Redis URL (required)")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "n", 4, "Number of parallel task processors (how many DNS lookups to process simultaneously)")
	cmd.Flags().IntVarP(&metricsPort, "metrics-port", "m", 9091, "Port for Prometheus metrics endpoint (if enabled)")
	cmd.Flags().BoolVarP(&enableMetrics, "enable-metrics", "M", false, "Enable metrics HTTP endpoint (useful for single worker, avoid port conflicts with multiple workers)")

	// DNS configuration
	cmd.Flags().IntVarP(&dnsTimeout, "dns-timeout", "T", 0, "DNS query timeout in seconds (default: from config or 5)")
	cmd.Flags().IntVarP(&maxConcurrentQueries, "max-concurrent", "C", 0, "Maximum concurrent DNS queries (default: from config or 500)")
	cmd.Flags().IntVarP(&maxRetries, "max-retries", "R", 0, "Number of retries per DNS query (default: from config or 3)")

	_ = cmd.MarkFlagRequired("redis")

	return cmd
}

func runWorker(cmd *cobra.Command, configPath, redisURL string, concurrency, metricsPort int, enableMetrics bool,
	dnsTimeout, maxConcurrentQueries, maxRetries int) error {

	// Load configuration
	if configPath == "" {
		configPath = "conf/config.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	if cmd.Flags().Changed("dns-timeout") {
		cfg.DNS.Timeout = dnsTimeout
	}
	if cmd.Flags().Changed("max-concurrent") {
		cfg.DNS.MaxConcurrentQueries = maxConcurrentQueries
	}
	if cmd.Flags().Changed("max-retries") {
		cfg.DNS.MaxRetries = maxRetries
	}
	if len(cfg.Servers) == 0 {
		slog.Warn("No DNS servers configured - worker will process tasks with explicit targets only", "path", configPath)
	} else {
		slog.Info("Configuration loaded", "path", configPath, "servers_count", len(cfg.Servers))
	}

	if redisURL == "" {
		slog.Error("Redis URL is required for worker")
		os.Exit(1)
	}

	redisAddr := redisURL
	if u, err := url.Parse(redisURL); err == nil {
		redisAddr = u.Host
	}

	// Start metrics server (optional)
	if enableMetrics {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			addr := fmt.Sprintf(":%d", metricsPort)
			slog.Info("Worker metrics server enabled", "address", addr)

			srv := &http.Server{
				Addr:         addr,
				Handler:      mux,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
			}

			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Metrics server error", "error", err)
			}
		}()
	} else {
		slog.Info("Worker metrics disabled (use --enable-metrics to enable)")
	}

	// Get DNS timeout from config
	dnsTimeoutDuration := time.Duration(cfg.GetDNSTimeout()) * time.Second
	slog.Info("DNS query timeout configured", "timeout", dnsTimeoutDuration)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer func() {
		if err := rdb.Close(); err != nil {
			slog.Error("Failed to close Redis connection", "error", err)
		}
	}()

	// Register handler with config closure
	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TaskTypeDNSLookup, func(ctx context.Context, t *asynq.Task) error {
		return handleTask(ctx, t, rdb, dnsTimeoutDuration, cfg)
	})

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: concurrency,
		},
	)

	// Run worker in background and wait for signal
	go func() {
		if err := srv.Run(mux); err != nil {
			slog.Error("Worker run failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	srv.Shutdown()
	return nil
}

// handleTask processes DNS lookup and stores result in Redis cache
func handleTask(ctx context.Context, t *asynq.Task, rdb *redis.Client, dnsTimeout time.Duration, cfg *config.APIConfig) error {
	var p map[string]interface{}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	taskID, _ := p["task_id"].(string)
	domain, _ := p["domain"].(string)
	qtype, _ := p["qtype"].(string)

	var servers []models.DNSServer
	if s, ok := p["servers"]; ok {
		b, _ := json.Marshal(s)
		_ = json.Unmarshal(b, &servers)
	}

	tlsInsecure, _ := p["tls_insecure"].(bool)

	start := time.Now()
	results := resolver.RunQueries(context.Background(), domain, qtype, servers, tlsInsecure, dnsTimeout, cfg.GetMaxConcurrentQueries(), cfg.GetMaxRetries())
	duration := time.Since(start).Seconds()

	// Build task metadata (Celery-style structure)
	taskMeta := map[string]interface{}{
		"status":  "SUCCESS",
		"task_id": taskID,
		"result": map[string]interface{}{
			"details":  results,
			"duration": duration,
		},
		"completed_at": time.Now().UTC(),
	}

	metaData, err := json.Marshal(taskMeta)
	if err != nil {
		slog.Error("Failed to marshal task metadata", "task_id", taskID, "error", err)
		return err
	}

	// Write to Redis cache (single key, fast reads)
	resultKey := fmt.Sprintf("dnstester:task-meta:%s", taskID)
	if err := rdb.Set(ctx, resultKey, metaData, 24*time.Hour).Err(); err != nil {
		slog.Error("Failed to cache result", "task_id", taskID, "error", err)
		return fmt.Errorf("failed to cache result: %w", err)
	}

	slog.Info("Task completed", "task_id", taskID, "duration_seconds", fmt.Sprintf("%.3f", duration))
	return nil
}
