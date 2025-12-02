package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudo-tiz/dns-tester-go/internal/app"
	"github.com/sudo-tiz/dns-tester-go/internal/config"
)

const (
	// DefaultMaxWorkers is the default number of worker goroutines
	DefaultMaxWorkers = 4
	// DefaultCleanupInterval is the default interval for cleaning up old tasks
	DefaultCleanupInterval = 10 * time.Minute
)

// NewServerCommand creates server subcommand with Cobra.
// Starts in-memory workers if Redis not configured.
func NewServerCommand() *cobra.Command {
	var configPath string
	var redisURL string
	var host string
	var port string
	var maxWorkers int

	// DNS config flags
	var dnsTimeout int
	var maxServersPerReq int
	var maxConcurrentQueries int
	var maxRetries int

	// Rate limiting flags
	var rateLimitRPS int
	var rateLimitBurst int

	// Server timeout flags
	var readTimeout int
	var writeTimeout int
	var idleTimeout int

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the DNS Tester API server",
		Long:  `Start the DNS Tester API server. Automatically starts in-memory workers if Redis is not configured.`,
		Example: `  # Start with default config
  dnstester server

  # Start with Redis backend
  dnstester server --redis redis://localhost:6379/0

  # Start with custom config
  dnstester server --config /path/to/config.yaml

  # Start on custom host/port
  dnstester server --host 0.0.0.0 --port 8080

  # Override DNS settings
  dnstester server --dns-timeout 10 --max-retries 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd, configPath, redisURL, host, port, maxWorkers,
				dnsTimeout, maxServersPerReq, maxConcurrentQueries, maxRetries,
				rateLimitRPS, rateLimitBurst, readTimeout, writeTimeout, idleTimeout)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", os.Getenv("CONFIG_PATH"), "Path to config file")
	cmd.Flags().StringVarP(&redisURL, "redis", "r", os.Getenv("REDIS_URL"), "Redis URL (optional, enables distributed workers)")
	cmd.Flags().StringVarP(&host, "host", "H", os.Getenv("DNS_TESTER_HOST"), "Server host (default: from config or 0.0.0.0)")
	cmd.Flags().StringVarP(&port, "port", "P", os.Getenv("DNS_TESTER_PORT"), "Server port (default: from config or 5000)")
	cmd.Flags().IntVarP(&maxWorkers, "workers", "w", 0, "Maximum number of workers (default: from config or 4)")

	// DNS configuration
	cmd.Flags().IntVar(&dnsTimeout, "dns-timeout", 0, "DNS query timeout in seconds (default: from config or 5)")
	cmd.Flags().IntVar(&maxServersPerReq, "max-servers", 0, "Maximum DNS servers per request (default: from config or 50)")
	cmd.Flags().IntVar(&maxConcurrentQueries, "max-concurrent", 0, "Maximum concurrent DNS queries (default: from config or 500)")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 0, "Number of retries per DNS query (default: from config or 3)")

	// Rate limiting
	cmd.Flags().IntVar(&rateLimitRPS, "rate-limit-rps", 0, "Rate limit requests per second (0 = disable, default: from config or 10)")
	cmd.Flags().IntVar(&rateLimitBurst, "rate-limit-burst", 0, "Rate limit burst size (default: from config or 20)")

	// HTTP server timeouts
	cmd.Flags().IntVar(&readTimeout, "read-timeout", 0, "HTTP read timeout in seconds (default: from config or 15)")
	cmd.Flags().IntVar(&writeTimeout, "write-timeout", 0, "HTTP write timeout in seconds (default: from config or 15)")
	cmd.Flags().IntVar(&idleTimeout, "idle-timeout", 0, "HTTP idle timeout in seconds (default: from config or 60)")

	return cmd
}

func runServer(cmd *cobra.Command, configPath, redisURL, host, port string, maxWorkers,
	dnsTimeout, maxServersPerReq, maxConcurrentQueries, maxRetries,
	rateLimitRPS, rateLimitBurst, readTimeout, writeTimeout, idleTimeout int) error {

	// Load config
	if configPath == "" {
		configPath = "conf/config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Apply basic overrides
	if host != "" {
		cfg.Server.Host = host
	}
	if port != "" {
		cfg.Server.Port = port
	}

	// Apply CLI flag overrides with defaults
	config.ApplyIntOverride(cmd.Flags().Changed("workers"), maxWorkers, &cfg.Worker.MaxWorkers, 4)
	config.ApplyIntOverride(cmd.Flags().Changed("dns-timeout"), dnsTimeout, &cfg.DNS.Timeout, 5)
	config.ApplyIntOverride(cmd.Flags().Changed("max-servers"), maxServersPerReq, &cfg.DNS.MaxServersPerReq, 50)
	config.ApplyIntOverride(cmd.Flags().Changed("max-concurrent"), maxConcurrentQueries, &cfg.DNS.MaxConcurrentQueries, 500)
	config.ApplyIntOverride(cmd.Flags().Changed("max-retries"), maxRetries, &cfg.DNS.MaxRetries, 3)
	config.ApplyIntOverride(cmd.Flags().Changed("rate-limit-rps"), rateLimitRPS, &cfg.RateLimiting.RequestsPerSecond, 10)
	config.ApplyIntOverride(cmd.Flags().Changed("rate-limit-burst"), rateLimitBurst, &cfg.RateLimiting.BurstSize, 20)
	config.ApplyIntOverride(cmd.Flags().Changed("read-timeout"), readTimeout, &cfg.Server.ReadTimeout, 15)
	config.ApplyIntOverride(cmd.Flags().Changed("write-timeout"), writeTimeout, &cfg.Server.WriteTimeout, 15)
	config.ApplyIntOverride(cmd.Flags().Changed("idle-timeout"), idleTimeout, &cfg.Server.IdleTimeout, 60)

	config.ApplyStringOverride(host, &cfg.Server.Host, "0.0.0.0")
	config.ApplyStringOverride(port, &cfg.Server.Port, "5000")

	// Log configuration status
	if len(cfg.Servers) == 0 {
		slog.Warn("No DNS servers configured - API will work but DNS lookups will require explicit targets", "path", configPath)
	} else {
		slog.Info("Configuration loaded", "path", configPath, "servers_count", len(cfg.Servers))
	}

	if redisURL == "" {
		slog.Info("Redis not configured - starting in memory mode (no task persistence)")
	} else {
		slog.Info("Redis configured", "url", redisURL)
	}

	// Create and start API app
	apiApp, err := app.NewAPIApp(cfg, redisURL)
	if err != nil {
		slog.Error("Failed to create API app", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := apiApp.Shutdown(context.Background()); err != nil {
			slog.Error("API app shutdown error", "error", err)
		}
	}()

	// Resolve address and start server
	if host == "" {
		host = cfg.GetServerHost()
	}
	if port == "" {
		port = cfg.GetServerPort()
	}
	addr := host + ":" + port

	go func() {
		slog.Info("Starting DNS Tester API server", "address", addr)
		if err := apiApp.Run(addr); err != nil {
			slog.Error("API app run failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return apiApp.Shutdown(ctx)
}
