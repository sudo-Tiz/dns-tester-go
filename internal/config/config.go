// Package config loads YAML configuration and provides defaults.
// Delegates validation to normalize package for DNS-specific rules.
package config

import (
	"fmt"
	"os"

	"github.com/sudo-tiz/dns-tester-go/internal/normalize"
	"gopkg.in/yaml.v3"
)

// ServiceType maps config values to DNS protocol schemes.
type ServiceType string

const (
	// ServiceDo53UDP represents DNS over UDP (port 53)
	ServiceDo53UDP ServiceType = "do53/udp"
	// ServiceDo53TCP represents DNS over TCP (port 53)
	ServiceDo53TCP ServiceType = "do53/tcp"
	// ServiceDoT represents DNS-over-TLS (port 853)
	ServiceDoT ServiceType = "dot"
	// ServiceDoH represents DNS-over-HTTPS (port 443)
	ServiceDoH ServiceType = "doh"
	// ServiceDoQ represents DNS-over-QUIC (port 853)
	ServiceDoQ ServiceType = "doq"
)

// DNSServer represents server configuration with flexible IP/hostname support.
type DNSServer struct {
	IP       string        `yaml:"ip,omitempty"`
	Port     int           `yaml:"port,omitempty"`
	Hostname string        `yaml:"hostname,omitempty"`
	Services []ServiceType `yaml:"services"`
	Tags     []string      `yaml:"tags,omitempty"`
}

// APIConfig is the root configuration structure.
type APIConfig struct {
	Servers      []DNSServer     `yaml:"servers"`
	RateLimiting RateLimitConfig `yaml:"rate_limiting,omitempty"`
	Server       ServerConfig    `yaml:"server,omitempty"`
	Worker       WorkerConfig    `yaml:"worker,omitempty"`
	DNS          DNSConfig       `yaml:"dns,omitempty"`
}

// RateLimitConfig controls tollbooth rate limiting.
type RateLimitConfig struct {
	RequestsPerSecond int `yaml:"requests_per_second,omitempty"`
	BurstSize         int `yaml:"burst_size,omitempty"`
}

// ServerConfig controls HTTP server timeouts and binding.
type ServerConfig struct {
	Host         string `yaml:"host,omitempty"`
	Port         string `yaml:"port,omitempty"`
	ReadTimeout  int    `yaml:"read_timeout,omitempty"`
	WriteTimeout int    `yaml:"write_timeout,omitempty"`
	IdleTimeout  int    `yaml:"idle_timeout,omitempty"`
}

// WorkerConfig controls Asynq worker concurrency.
type WorkerConfig struct {
	MaxWorkers      int `yaml:"max_workers,omitempty"`
	CleanupInterval int `yaml:"cleanup_interval,omitempty"`
}

// DNSConfig controls DNS query behavior.
type DNSConfig struct {
	Timeout              int `yaml:"timeout,omitempty"`
	MaxServersPerReq     int `yaml:"max_servers_per_req,omitempty"`
	MaxConcurrentQueries int `yaml:"max_concurrent_queries,omitempty"`
	MaxRetries           int `yaml:"max_retries,omitempty"`
}

// Validate delegates IP validation to normalize.IsValidIP.
// Do53 requires IP (no hostname resolution) - pragmatic choice for UDP/TCP.
func (s *DNSServer) Validate() error {
	if s.IP == "" && s.Hostname == "" {
		return fmt.Errorf("at least one of 'ip' or 'hostname' must be provided")
	}

	if s.IP != "" {
		if !normalize.IsValidIP(s.IP) {
			return fmt.Errorf("invalid IP address: %s", s.IP)
		}
	}

	if s.Port != 0 && (s.Port < 1 || s.Port > 65535) {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", s.Port)
	}

	// Do53 needs IP - avoid DNS lookup for DNS server address
	for _, svc := range s.Services {
		if (svc == ServiceDo53UDP || svc == ServiceDo53TCP) && s.IP == "" {
			return fmt.Errorf("do53/udp and do53/tcp require an IP address (not just a hostname)")
		}
	}

	return nil
}

// LoadConfig reads YAML and validates servers.
// Returns empty config if file missing - optional config approach.
func LoadConfig(filePath string) (*APIConfig, error) {
	// #nosec G304 -- filePath is user-controlled via CLI flag by design
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &APIConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config APIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	for i, server := range config.Servers {
		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("server %d validation failed: %w", i, err)
		}
	}

	return &config, nil
}

// DNSTarget combines normalized target URL with tags.
type DNSTarget struct {
	Target string   `json:"target"`
	Tags   []string `json:"tags,omitempty"`
}

// GetDNSTargets transforms YAML config to normalized targets.
// normalize.ProtocolConfigs is single source of truth for scheme/port mapping.
func (c *APIConfig) GetDNSTargets() []DNSTarget {
	var targets []DNSTarget

	serviceToScheme := map[ServiceType]string{
		ServiceDo53UDP: normalize.SchemeUDP,
		ServiceDo53TCP: normalize.SchemeTCP,
		ServiceDoT:     normalize.SchemeTLS,
		ServiceDoH:     normalize.SchemeHTTPS,
		ServiceDoQ:     normalize.SchemeQUIC,
	}

	for _, server := range c.Servers {
		for _, svc := range server.Services {
			scheme, ok := serviceToScheme[svc]
			if !ok {
				continue
			}

			protoCfg, ok := normalize.ProtocolConfigs[scheme]
			if !ok {
				continue
			}

			// Use hostname for protocols that support it (DoT, DoH, DoQ)
			host := server.IP
			if protoCfg.UsesHostname && server.Hostname != "" {
				host = server.Hostname
			}

			port := server.Port
			if port == 0 {
				port = protoCfg.DefaultPort
			}

			raw := fmt.Sprintf("%s://%s:%d", protoCfg.Scheme, host, port)
			norm, err := normalize.Target(raw)
			if err != nil {
				continue
			}

			tags := server.Tags
			if tags == nil {
				tags = []string{}
			}

			targets = append(targets, DNSTarget{
				Target: norm,
				Tags:   tags,
			})
		}
	}

	return targets
}

// GetRateLimitRequestsPerSecond provides default fallback.
// Returns 0 if explicitly set to 0 (disables rate limiting).
func (c *APIConfig) GetRateLimitRequestsPerSecond() int {
	if c.RateLimiting.RequestsPerSecond >= 0 {
		return c.RateLimiting.RequestsPerSecond
	}
	return 10
}

// GetRateLimitBurstSize provides default fallback.
func (c *APIConfig) GetRateLimitBurstSize() int {
	if c.RateLimiting.BurstSize > 0 {
		return c.RateLimiting.BurstSize
	}
	return 20
}

// GetServerHost provides default fallback.
func (c *APIConfig) GetServerHost() string {
	if c.Server.Host != "" {
		return c.Server.Host
	}
	return "0.0.0.0"
}

// GetServerPort provides default fallback.
func (c *APIConfig) GetServerPort() string {
	if c.Server.Port != "" {
		return c.Server.Port
	}
	return "5000"
}

// GetServerReadTimeout provides default fallback (seconds).
func (c *APIConfig) GetServerReadTimeout() int {
	if c.Server.ReadTimeout > 0 {
		return c.Server.ReadTimeout
	}
	return 15
}

// GetServerWriteTimeout provides default fallback (seconds).
func (c *APIConfig) GetServerWriteTimeout() int {
	if c.Server.WriteTimeout > 0 {
		return c.Server.WriteTimeout
	}
	return 15
}

// GetServerIdleTimeout provides default fallback (seconds).
func (c *APIConfig) GetServerIdleTimeout() int {
	if c.Server.IdleTimeout > 0 {
		return c.Server.IdleTimeout
	}
	return 60
}

// GetMaxWorkers provides default fallback.
func (c *APIConfig) GetMaxWorkers() int {
	if c.Worker.MaxWorkers > 0 {
		return c.Worker.MaxWorkers
	}
	return 4
}

// GetWorkerCleanupInterval provides default fallback (minutes).
func (c *APIConfig) GetWorkerCleanupInterval() int {
	if c.Worker.CleanupInterval > 0 {
		return c.Worker.CleanupInterval
	}
	return 10
}

// GetDNSTimeout provides default fallback (seconds).
func (c *APIConfig) GetDNSTimeout() int {
	if c.DNS.Timeout > 0 {
		return c.DNS.Timeout
	}
	return 5
}

// GetMaxServersPerRequest provides default fallback.
func (c *APIConfig) GetMaxServersPerRequest() int {
	if c.DNS.MaxServersPerReq > 0 {
		return c.DNS.MaxServersPerReq
	}
	return 50
}

// GetMaxConcurrentQueries provides default fallback.
func (c *APIConfig) GetMaxConcurrentQueries() int {
	if c.DNS.MaxConcurrentQueries > 0 {
		return c.DNS.MaxConcurrentQueries
	}
	return 500
}

// GetMaxRetries provides default fallback.
func (c *APIConfig) GetMaxRetries() int {
	if c.DNS.MaxRetries > 0 {
		return c.DNS.MaxRetries
	}
	return 3
}

// ApplyIntOverride applies a CLI flag override to a config int field with default fallback.
// If the CLI flag was changed and the value is positive, it overrides the config value.
// Otherwise, if the config value is zero, the default value is applied.
func ApplyIntOverride(flagChanged bool, flagValue int, target *int, defaultVal int) {
	if flagChanged && flagValue > 0 {
		*target = flagValue
	} else if *target == 0 {
		*target = defaultVal
	}
}

// ApplyStringOverride applies a CLI flag override to a config string field with default fallback.
// If the CLI value is non-empty, it overrides the config value.
// Otherwise, if the config value is empty, the default value is applied.
func ApplyStringOverride(cliValue string, target *string, defaultVal string) {
	if cliValue != "" {
		*target = cliValue
	} else if *target == "" {
		*target = defaultVal
	}
}
