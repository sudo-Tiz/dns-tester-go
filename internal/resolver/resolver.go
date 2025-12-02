// Package resolver performs DNS queries using AdGuard dnsproxy for multi-protocol support.
// Delegates protocol handling (Do53, DoT, DoH, DoQ) to AdGuard upstream library.
package resolver

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/sudo-tiz/dns-tester-go/internal/metrics"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
	"github.com/sudo-tiz/dns-tester-go/internal/normalize"
)

const (
	// CommandStatusOK indicates a successful DNS query
	CommandStatusOK = "ok"
	// CommandStatusError indicates a failed DNS query
	CommandStatusError = "error"

	// DefaultTimeout is the default timeout for DNS queries
	DefaultTimeout = 5 * time.Second
	// RetryDelay is the brief delay between retries
	RetryDelay = 100 * time.Millisecond // Brief delay between retries to avoid hammering
)

// RCodeMapping uses miekg/dns constants for response codes.
var RCodeMapping = map[int]string{
	dns.RcodeSuccess:        "NOERROR",
	dns.RcodeFormatError:    "FORMERR",
	dns.RcodeServerFailure:  "SERVFAIL",
	dns.RcodeNameError:      "NXDOMAIN",
	dns.RcodeNotImplemented: "NOTIMP",
	dns.RcodeRefused:        "REFUSED",
}

// GetDNSProtocolFromTarget extracts display name from normalize.ProtocolConfigs.
func GetDNSProtocolFromTarget(target string) string {
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" {
		return "Unknown"
	}

	if cfg, ok := normalize.ProtocolConfigs[u.Scheme]; ok {
		return cfg.DisplayName
	}

	return "Unknown"
}

// stringToQType delegates to miekg/dns.StringToType to avoid maintaining type list.
func stringToQType(qtype string) (uint16, error) {
	if dnsType, ok := dns.StringToType[strings.ToUpper(qtype)]; ok {
		return dnsType, nil
	}
	return 0, fmt.Errorf("unsupported query type: %s", qtype)
}

// qtypeToString uses miekg/dns reverse mapping.
func qtypeToString(qtype uint16) string {
	if s, ok := dns.TypeToString[qtype]; ok {
		return s
	}
	return fmt.Sprintf("TYPE%d", qtype)
}

// QueryServer performs DNS query via AdGuard dnsproxy with retry logic.
// Retries 3 times with 100ms delay - pragmatic default for transient network issues.
func QueryServer(ctx context.Context, domain, qtype string, server models.DNSServer, tlsInsecure bool, retries int, timeout time.Duration) (string, models.DNSLookupResult) {
	result := models.DNSLookupResult{
		Tags:        server.Tags,
		DNSProtocol: GetDNSProtocolFromTarget(server.Target),
	}

	dnsType, err := stringToQType(qtype)
	if err != nil {
		result.CommandStatus = CommandStatusError
		result.Error = err.Error()
		metrics.DNSLookupErrors.WithLabelValues(server.Target, "invalid_qtype").Inc()
		return server.Target, result
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dnsType)
	msg.RecursionDesired = true

	var response *dns.Msg
	var rtt time.Duration

	for attempt := 0; attempt < retries; attempt++ {
		select {
		case <-ctx.Done():
			result.CommandStatus = CommandStatusError
			result.Error = fmt.Sprintf("context cancelled: %v", ctx.Err())
			metrics.DNSLookupErrors.WithLabelValues(server.Target, "context_cancelled").Inc()
			return server.Target, result
		default:
		}

		response, rtt, err = performQuery(ctx, msg, server.Target, tlsInsecure, timeout)

		if err == nil && response != nil {
			break
		}

		if ctx.Err() != nil {
			result.CommandStatus = CommandStatusError
			result.Error = fmt.Sprintf("context cancelled: %v", ctx.Err())
			metrics.DNSLookupErrors.WithLabelValues(server.Target, "context_cancelled").Inc()
			return server.Target, result
		}

		if attempt < retries-1 {
			time.Sleep(RetryDelay)
		}
	}

	if err != nil {
		result.CommandStatus = CommandStatusError
		result.Error = fmt.Sprintf("query failed: %v", err)
		metrics.DNSLookupErrors.WithLabelValues(server.Target, "query_failed").Inc()
		return server.Target, result
	}

	if response == nil {
		result.CommandStatus = CommandStatusError
		result.Error = "no response received"
		metrics.DNSLookupErrors.WithLabelValues(server.Target, "no_response").Inc()
		return server.Target, result
	}

	result.CommandStatus = CommandStatusOK
	result.TimeMs = float64(rtt.Microseconds()) / 1000.0
	result.RCode = RCodeMapping[response.Rcode]
	if result.RCode == "" {
		result.RCode = fmt.Sprintf("UNKNOWN(%d)", response.Rcode)
	}

	metrics.RecordQueryMetrics(server.Target, result.TimeMs/1000.0, result.RCode, qtype)

	if len(response.Question) > 0 {
		result.Name = strings.TrimSuffix(response.Question[0].Name, ".")
		result.QType = qtypeToString(response.Question[0].Qtype)
	}

	// Parse answers using miekg/dns type assertions
	result.Answers = []models.DNSAnswer{}
	for _, rr := range response.Answer {
		answer := models.DNSAnswer{
			Name: strings.TrimSuffix(rr.Header().Name, "."),
			Type: qtypeToString(rr.Header().Rrtype),
			TTL:  rr.Header().Ttl,
		}

		// Type switch instead of reflection for performance
		switch v := rr.(type) {
		case *dns.A:
			answer.Value = v.A.String()
		case *dns.AAAA:
			answer.Value = v.AAAA.String()
		case *dns.CNAME:
			answer.Value = strings.TrimSuffix(v.Target, ".")
		case *dns.MX:
			answer.Value = fmt.Sprintf("%d %s", v.Preference, strings.TrimSuffix(v.Mx, "."))
		case *dns.NS:
			answer.Value = strings.TrimSuffix(v.Ns, ".")
		case *dns.PTR:
			answer.Value = strings.TrimSuffix(v.Ptr, ".")
		case *dns.TXT:
			answer.Value = strings.Join(v.Txt, " ")
		case *dns.SOA:
			answer.Value = fmt.Sprintf("%s %s %d %d %d %d %d",
				strings.TrimSuffix(v.Ns, "."),
				strings.TrimSuffix(v.Mbox, "."),
				v.Serial, v.Refresh, v.Retry, v.Expire, v.Minttl)
		case *dns.SRV:
			answer.Value = fmt.Sprintf("%d %d %d %s",
				v.Priority, v.Weight, v.Port, strings.TrimSuffix(v.Target, "."))
		case *dns.CAA:
			answer.Value = fmt.Sprintf("%d %s %s", v.Flag, v.Tag, v.Value)
		default:
			answer.Value = rr.String()
		}

		result.Answers = append(result.Answers, answer)
	}

	return server.Target, result
}

// performQuery delegates DNS query execution to AdGuard upstream library.
// Target must be prenormalized - passed directly to AdGuard for protocol handling.
func performQuery(ctx context.Context, msg *dns.Msg, normalizedTarget string, tlsInsecure bool, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	start := time.Now()

	opts := &upstream.Options{
		Timeout: timeout,
	}
	if tlsInsecure {
		// #nosec G402 - user-controlled for testing encrypted protocols
		slog.Warn("TLS certificate verification is DISABLED - USE ONLY FOR TESTING",
			"target", normalizedTarget)
		opts.InsecureSkipVerify = true
	}

	// AdGuard upstream.AddressToUpstream handles scheme parsing, port defaults, IPv6 brackets
	up, err := upstream.AddressToUpstream(normalizedTarget, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create upstream: %w", err)
	}
	defer func() {
		_ = up.Close()
	}()

	// Run Exchange in goroutine to enable context cancellation
	type result struct {
		resp *dns.Msg
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		resp, err := up.Exchange(msg)
		resultCh <- result{resp: resp, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, 0, fmt.Errorf("query cancelled: %w", ctx.Err())
	case res := <-resultCh:
		if res.err != nil {
			return nil, 0, fmt.Errorf("DNS query failed: %w", res.err)
		}
		rtt := time.Since(start)
		return res.resp, rtt, nil
	}
}

// RunQueries fans out queries to multiple servers with concurrency limit.
// Semaphore pattern prevents resource exhaustion when querying many servers.
func RunQueries(ctx context.Context, domain, qtype string, servers []models.DNSServer, tlsInsecure bool, timeout time.Duration, maxConcurrentQueries, maxRetries int) map[string]models.DNSLookupResult {
	results := make(map[string]models.DNSLookupResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	pool := make(chan struct{}, maxConcurrentQueries)

	for _, server := range servers {
		wg.Add(1)
		pool <- struct{}{}

		go func(srv models.DNSServer) {
			defer wg.Done()
			defer func() { <-pool }()

			target, result := QueryServer(ctx, domain, qtype, srv, tlsInsecure, maxRetries, timeout)
			mu.Lock()
			results[target] = result
			mu.Unlock()
		}(server)
	}

	wg.Wait()
	return results
}
