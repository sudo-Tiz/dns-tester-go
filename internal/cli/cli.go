// Package cli provides command-line interface for DNS Tester GO.
package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudo-tiz/dns-tester-go/internal/api"
	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
	"github.com/sudo-tiz/dns-tester-go/internal/normalize"
)

const (
	// PackageVersion is the current version of the CLI
	PackageVersion = "1.0.0"

	// DefaultAPIURL is the default API server URL
	DefaultAPIURL = "http://localhost:5000"
	// DefaultQType is the default DNS query type
	DefaultQType = "A"
	// QTypePTR is the PTR (reverse DNS) query type
	QTypePTR = "PTR"
	// DefaultWarnThreshold is the default response time warning threshold in seconds
	DefaultWarnThreshold = 1.0
	// DefaultPollInterval is the default interval for polling task status
	DefaultPollInterval = 500 * time.Millisecond
)

const (
	levelInfo = "ok"
	levelWarn = "warn"
	levelErr  = "error"
)

var (
	apiURL        string
	qtype         string
	insecure      bool
	debug         bool
	pretty        bool
	warnThreshold float64
	dnsServers    []string
)

// NewRootCmd creates the root CLI command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "dnstestergo",
		Short:   "DNS testing tool with support for Do53, DoT, DoH, DoQ",
		Long:    `A comprehensive DNS testing tool that supports multiple protocols including UDP/TCP (Do53), DNS-over-TLS (DoT), DNS-over-HTTPS (DoH), and DNS-over-QUIC (DoQ).`,
		Version: PackageVersion,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("at least one argument is required")
			}
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&apiURL, "api-url", "u", DefaultAPIURL, "Base URL of the API")
	rootCmd.Flags().StringVarP(&qtype, "qtype", "t", DefaultQType, "DNS query type (A, AAAA, PTR)")
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "i", false, "Skip TLS certificate verification")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Show detailed error messages for failed lookups")
	rootCmd.Flags().BoolVarP(&pretty, "pretty", "p", false, "Enable emoji-enhanced output")
	rootCmd.Flags().Float64VarP(&warnThreshold, "warn-threshold", "w", DefaultWarnThreshold, "Response time threshold in seconds for warnings")

	rootCmd.AddCommand(NewQueryCommand())
	rootCmd.AddCommand(NewServerCommand())
	rootCmd.AddCommand(NewWorkerCommand())
	return rootCmd
}

// buildDNSServers converts server targets to DNSServer models.
func buildDNSServers(servers []string) []models.DNSServer {
	result := make([]models.DNSServer, 0, len(servers))
	for _, s := range servers {
		result = append(result, models.DNSServer{Target: s})
	}
	return result
}

// NewQueryCommand creates the 'query' subcommand.
func NewQueryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query [domain] [dns_servers...]",
		Aliases: []string{"q", "lookup"},
		Short:   "Perform DNS queries",
		Long:    `Perform DNS queries against one or more DNS servers with support for multiple protocols (UDP, TCP, DoT, DoH, DoQ).`,
		Example: `  # Query using UDP
  dnstestergo query github.com udp://9.9.9.9:53

  # Query using DNS-over-HTTPS
  dnstestergo query github.com https://dns.quad9.net/dns-query

  # Query using DNS-over-QUIC
  dnstestergo query github.com quic://dns.adguard-dns.com:853

  # Query with multiple servers
  dnstestergo query example.com udp://9.9.9.9:53 tls://94.140.14.14:853

  # Reverse DNS lookup
  dnstestergo query -r 9.9.9.9

  # Custom query type
  dnstestergo query --qtype=AAAA github.com udp://9.9.9.9:53`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runDNSTest(cmd, args)
			if err != nil {
				// Affiche seulement l'erreur, sans le help
				cmd.PrintErrln(err)
				return nil
			}
			return nil
		},
	}

	// Reuse existing flags from the original CLI
	cmd.Flags().StringVarP(&apiURL, "api-url", "u", DefaultAPIURL, "Base URL of the API")
	cmd.Flags().StringVarP(&qtype, "qtype", "t", DefaultQType, "DNS query type (A, AAAA, PTR)")
	cmd.Flags().BoolVarP(&insecure, "insecure", "i", false, "Skip TLS certificate verification")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Show detailed error messages for failed lookups")
	cmd.Flags().BoolVarP(&pretty, "pretty", "p", false, "Enable emoji-enhanced output")
	cmd.Flags().Float64VarP(&warnThreshold, "warn-threshold", "w", DefaultWarnThreshold, "Response time threshold in seconds for warnings")
	var configPath string
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")

	return cmd
}

func runDNSTest(_ *cobra.Command, args []string) error {
	var query string
	if len(args) > 0 {
		query = args[0]
	}

	configPath := ""
	for _, f := range os.Args {
		if strings.HasPrefix(f, "--config=") {
			configPath = strings.TrimPrefix(f, "--config=")
		}
	}
	if configPath == "" {
		for i, f := range os.Args {
			if f == "--config" && i+1 < len(os.Args) {
				configPath = os.Args[i+1]
			}
		}
	}

	dnsServers = nil
	if len(args) > 1 {
		dnsServers = args[1:]
	}

	if configPath != "" {
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("erreur chargement config: %w", err)
		}
		dnsServers = nil
		for _, t := range cfg.GetDNSTargets() {
			dnsServers = append(dnsServers, t.Target)
		}
		if len(dnsServers) == 0 {
			return fmt.Errorf("aucun serveur DNS trouvé dans la config")
		}
	}

	for _, server := range dnsServers {
		if err := validateAddress(server); err != nil {
			return fmt.Errorf("error: %w", err)
		}
	}

	// Auto-detect PTR (reverse) lookup if query is an IP
	queryType := qtype
	domain := query
	if normalize.IsValidIP(query) {
		fmt.Printf("Starting Reverse DNS lookup for IP: %s ", query)
		queryType = QTypePTR
		// Convert IP to reverse DNS format
		reverseDomain, err := normalize.IPToReverseDNS(query)
		if err != nil {
			return fmt.Errorf("error converting IP to reverse format: %w", err)
		}
		domain = reverseDomain
	} else {
		fmt.Printf("Starting DNS lookup for domain: %s ", query)
	}

	if debug {
		fmt.Printf("\n\tUsing DNS servers: %s\n", strings.Join(dnsServers, ", "))
		fmt.Printf("\tQuery type: %s\n", queryType)
		if queryType == QTypePTR {
			fmt.Printf("\tReverse domain: %s\n", domain)
		}
		fmt.Printf("\tAPI Base URL: %s\n", apiURL)
		fmt.Printf("\tTLS Skip Verify: %t\n", insecure)
		if insecure {
			fmt.Println("\t⚠️  WARNING: TLS certificate verification is DISABLED - USE ONLY FOR TESTING")
		}
	}

	// Post lookup request using API client
	ctx := context.Background()
	client := api.NewClient(apiURL, 30*time.Second, insecure)
	dnsServersModel := buildDNSServers(dnsServers)
	taskID, err := client.EnqueueDNSLookup(ctx, models.DNSLookupRequest{
		Domain:                domain,
		DNSServers:            dnsServersModel,
		QType:                 queryType,
		TLSInsecureSkipVerify: insecure,
	})
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if debug {
		fmt.Printf("\tTask ID: %s\n", taskID)
	}

	// Poll for task completion
	for {
		taskStatus, err := client.GetTaskStatus(ctx, taskID)
		if err != nil {
			return fmt.Errorf("error: %w", err)
		}

		if taskStatus.Status == "SUCCESS" {
			printResults(taskStatus, queryType == QTypePTR, queryType)
			break
		} else if taskStatus.Status == "FAILURE" {
			fmt.Println("\n\tTask failed.")
			break
		}

		fmt.Print(".")
		time.Sleep(DefaultPollInterval)
	}

	return nil
}

// HTTP helper functions removed — CLI now uses internal/api Client.

func printResults(taskStatus *models.TaskStatusResponse, isReverse bool, queryType string) {
	if taskStatus.Result == nil {
		fmt.Println("\nNo results available")
		return
	}

	nbCommandsOK := 0
	for _, result := range taskStatus.Result.Details {
		if result.CommandStatus == "ok" {
			nbCommandsOK++
		}
	}

	nbCommands := len(taskStatus.Result.Details)
	totalDuration := taskStatus.Result.Duration

	fmt.Printf("\nDNS lookup succeeded for %d out of %d servers (%.4f seconds total)\n",
		nbCommandsOK, nbCommands, totalDuration)

	// Sort results
	type sortedResult struct {
		server string
		result models.DNSLookupResult
	}
	var sorted []sortedResult
	for server, result := range taskStatus.Result.Details {
		sorted = append(sorted, sortedResult{server, result})
	}
	sort.Slice(sorted, func(i, j int) bool {
		hostI := extractHost(sorted[i].server)
		hostJ := extractHost(sorted[j].server)
		if hostI != hostJ {
			return hostI < hostJ
		}
		protoI := sorted[i].result.DNSProtocol
		protoJ := sorted[j].result.DNSProtocol
		if protoI == "" {
			protoI = "unknown"
		}
		if protoJ == "" {
			protoJ = "unknown"
		}
		return protoI < protoJ
	})

	for _, item := range sorted {
		server := item.server
		result := item.result

		if result.CommandStatus == "ok" {
			dnsProtocol := result.DNSProtocol
			rcode := result.RCode
			if rcode == "" {
				rcode = "Unknown"
			}

			if rcode != "NOERROR" {
				if rcode == "NXDOMAIN" {
					logResult("warn", fmt.Sprintf("%s - Domain does not exist (rcode: NXDOMAIN) - %.2f ms",
						server, result.TimeMs))
				} else {
					logResult("warn", fmt.Sprintf("%s - No valid answer (rcode: %s) - %.2f ms",
						server, rcode, result.TimeMs))
				}
			} else {
				recordType := queryType
				if isReverse {
					recordType = QTypePTR
				}

				// Filter answers by record type
				var answers []models.DNSAnswer
				for _, ans := range result.Answers {
					if ans.Type == recordType {
						answers = append(answers, ans)
					}
				}

				if len(answers) > 0 {
					var values []string
					var ttls []uint32
					for _, ans := range answers {
						values = append(values, ans.Value)
						ttls = append(ttls, ans.TTL)
					}

					timeMs := result.TimeMs
					timeSec := timeMs / 1000.0

					// Determine log level based on threshold
					level := levelInfo
					if timeSec > warnThreshold {
						level = levelWarn
					} // Check if all TTLs are the same
					allSameTTL := true
					if len(ttls) > 1 {
						for i := 1; i < len(ttls); i++ {
							if ttls[i] != ttls[0] {
								allSameTTL = false
								break
							}
						}
					}

					if allSameTTL {
						logResult(level, fmt.Sprintf("%s - %s - %.5fms - TTL: %ds - %s",
							server, dnsProtocol, timeMs, ttls[0], strings.Join(values, ", ")))
					} else {
						var valueWithTTL []string
						for _, ans := range answers {
							valueWithTTL = append(valueWithTTL, fmt.Sprintf("%s (TTL: %d)", ans.Value, ans.TTL))
						}
						logResult(level, fmt.Sprintf("%s - %s - %.5fms - %s",
							server, dnsProtocol, timeMs, strings.Join(valueWithTTL, ", ")))
					}
				} else {
					logResult(levelWarn, fmt.Sprintf("%s - %s - No %s records found - %.2f ms",
						server, dnsProtocol, recordType, result.TimeMs))
				}
			}
		} else {
			if debug {
				logResult(levelErr, fmt.Sprintf("%s - connection issue or error: %s", server, result.Error))
			} else {
				logResult(levelErr, fmt.Sprintf("%s - connection issue or error", server))
			}
		}
	}
}

func logResult(level, message string) {
	symbols := map[string][2]string{
		"ok":    {"✅ ", "[OK] "},
		"warn":  {"⚠️ ", "[WARN] "},
		"error": {"❌ ", "[FAILED] "},
	}

	symbol := "[???] "
	if syms, ok := symbols[level]; ok {
		if pretty {
			symbol = syms[0]
		} else {
			symbol = syms[1]
		}
	}

	fmt.Printf("%s%s\n", symbol, message)
}

func validateAddress(serverAddress string) error {
	if _, err := normalize.Target(serverAddress); err != nil {
		return fmt.Errorf("invalid server address format: %w", err)
	}
	return nil
}

func extractHost(target string) string {
	// Parse URL to extract host (AdGuard dnsproxy will handle the full target)
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		// If parsing fails or no host, return as-is (might be plain IP)
		return target
	}
	return u.Host
}

// Execute runs the CLI
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewRootCommand is an alias for backward compatibility.
func NewRootCommand() *cobra.Command {
	return NewRootCmd()
}
