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
  dnstester worker --redis redis://localhost:6379/0

  # Start worker with custom concurrency
  dnstester worker --redis redis://localhost:6379/0 --concurrency 8

  # Start worker with metrics enabled (useful for single worker or dev)
  dnstester worker --config /path/to/config.yaml --redis redis://localhost:6379/0 --enable-metrics

  # Override DNS settings
  dnstester worker --redis redis://localhost:6379/0 --dns-timeout 10 --max-retries 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorker(cmd, configPath, redisURL, concurrency, metricsPort, enableMetrics,
				dnsTimeout, maxConcurrentQueries, maxRetries)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", os.Getenv("CONFIG_PATH"), "Path to config file")
	cmd.Flags().StringVarP(&redisURL, "redis", "r", os.Getenv("REDIS_URL"), "Redis URL (required)")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "n", 4, "Number of concurrent workers")
	cmd.Flags().IntVarP(&metricsPort, "metrics-port", "m", 9091, "Port for Prometheus metrics endpoint (if enabled)")
	cmd.Flags().BoolVarP(&enableMetrics, "enable-metrics", "M", false, "Enable metrics HTTP endpoint (useful for single worker, avoid port conflicts with multiple workers)")

	// DNS configuration
	cmd.Flags().IntVar(&dnsTimeout, "dns-timeout", 0, "DNS query timeout in seconds (default: from config or 5)")
	cmd.Flags().IntVar(&maxConcurrentQueries, "max-concurrent", 0, "Maximum concurrent DNS queries (default: from config or 500)")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 0, "Number of retries per DNS query (default: from config or 3)")

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

	// Apply CLI overrides to DNS config with defaults
	config.ApplyIntOverride(cmd.Flags().Changed("dns-timeout"), dnsTimeout, &cfg.DNS.Timeout, 5)
	config.ApplyIntOverride(cmd.Flags().Changed("max-concurrent"), maxConcurrentQueries, &cfg.DNS.MaxConcurrentQueries, 500)
	config.ApplyIntOverride(cmd.Flags().Changed("max-retries"), maxRetries, &cfg.DNS.MaxRetries, 3)

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

	// Register handler with config closure
	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TaskTypeDNSLookup, func(ctx context.Context, t *asynq.Task) error {
		return handleTask(ctx, t, dnsTimeoutDuration, cfg)
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

// handleTask processes a task and stores result using Asynq's native ResultWriter
// This eliminates the need for custom Redis storage and prevents race conditions
func handleTask(_ context.Context, t *asynq.Task, dnsTimeout time.Duration, cfg *config.APIConfig) error {
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

	res := map[string]interface{}{
		"details":  results,
		"duration": duration,
	}

	resultData, err := json.Marshal(res)
	if err != nil {
		slog.Error("Failed to marshal result", "task_id", taskID, "error", err)
		return err
	}

	if _, err := t.ResultWriter().Write(resultData); err != nil {
		slog.Error("Failed to write result", "task_id", taskID, "error", err)
		return fmt.Errorf("failed to write result: %w", err)
	}

	slog.Info("Task completed", "task_id", taskID, "duration_seconds", fmt.Sprintf("%.3f", duration))
	return nil
}
