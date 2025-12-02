// Package api provides HTTP API server for DNS lookups.
// Uses chi router, tollbooth rate limiting, and Prometheus metrics.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/didip/tollbooth/v8"
	"github.com/didip/tollbooth/v8/limiter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sudo-tiz/dns-tester-go/internal/config"
	"github.com/sudo-tiz/dns-tester-go/internal/metrics"
	"github.com/sudo-tiz/dns-tester-go/internal/models"
	"github.com/sudo-tiz/dns-tester-go/internal/normalize"
	"github.com/sudo-tiz/dns-tester-go/internal/tasks"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "github.com/sudo-tiz/dns-tester-go/internal/api/docs" // swagger docs
)

// APIVersion is the current version of the API
const APIVersion = "1.0.0"

// Server wraps chi router with task queue client for async DNS lookups.
type Server struct {
	router      *chi.Mux
	config      *config.APIConfig
	tasksClient tasks.ClientInterface
}

// NewServer configures middleware stack: tollbooth, chi logging, panic recovery.
func NewServer(cfg *config.APIConfig) *Server {
	s := &Server{router: chi.NewRouter(), config: cfg}

	// Tollbooth rate limiter with configurable IP source (RemoteAddr, X-Forwarded-For, etc.)
	// Only enable if RequestsPerSecond > 0 (0 = disabled)
	if cfg.RateLimiting.RequestsPerSecond > 0 {
		lmt := tollbooth.NewLimiter(
			float64(cfg.GetRateLimitRequestsPerSecond()),
			&limiter.ExpirableOptions{DefaultExpirationTTL: 10 * time.Minute},
		)
		lmt.SetBurst(cfg.GetRateLimitBurstSize())

		ipSource := os.Getenv("RATE_LIMIT_IP_SOURCE")
		if ipSource == "" {
			ipSource = "RemoteAddr"
		}
		lmt.SetIPLookup(limiter.IPLookup{Name: ipSource, IndexFromRight: 0})
		lmt.SetMessage(`{"error":"rate limit exceeded"}`)
		lmt.SetMessageContentType("application/json")

		s.router.Use(func(next http.Handler) http.Handler {
			return tollbooth.HTTPMiddleware(lmt)(next)
		})
	}

	// Chi middleware for logging, recovery, request ID, real IP
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)

	s.router.Post("/dns-lookup", s.handleDNSLookup)
	s.router.Post("/reverse-lookup", s.handleReverseLookup)
	s.router.Get("/tasks/{taskID}", s.handleGetTaskStatus)
	s.router.Get("/health", s.handleHealthCheck)
	s.router.Head("/health", s.handleHealthCheck)
	s.router.Get("/status", s.handleHealthCheck) // Python dnstester compat
	s.router.Head("/status", s.handleHealthCheck)
	s.router.Get("/metrics", s.handleMetrics)

	// Swagger UI and OpenAPI endpoints
	s.router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/index.html", http.StatusMovedPermanently)
	})
	s.router.Get("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	))
	return s
}

// SetTasksClient injects task queue client (Asynq or in-memory).
func (s *Server) SetTasksClient(c tasks.ClientInterface) { s.tasksClient = c }

// Router exposes chi.Mux for testing.
func (s *Server) Router() http.Handler { return s.router }

// Run starts HTTP server with config-driven timeouts.
func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  time.Duration(s.config.GetServerReadTimeout()) * time.Second,
		WriteTimeout: time.Duration(s.config.GetServerWriteTimeout()) * time.Second,
		IdleTimeout:  time.Duration(s.config.GetServerIdleTimeout()) * time.Second,
	}
	return srv.ListenAndServe()
}

// handleDNSLookup submits a DNS lookup task for asynchronous processing
// @Summary Submit DNS lookup task
// @Description Enqueue a DNS lookup for asynchronous processing. Returns a task ID that can be polled.
// @Tags DNS
// @Accept json
// @Produce json
// @Param request body models.DNSLookupRequest true "DNS lookup parameters"
// @Success 200 {object} models.TaskResponse "Task accepted and enqueued"
// @Failure 400 {object} models.ErrorResponse "Invalid request or missing parameters"
// @Failure 429 {object} models.ErrorResponse "Rate limit exceeded"
// @Failure 503 {object} models.ErrorResponse "No workers available"
// @Router /dns-lookup [post]
func (s *Server) handleDNSLookup(w http.ResponseWriter, r *http.Request) {
	var req models.DNSLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	metrics.APIRequestsTotal.WithLabelValues("dns-lookup").Inc()
	s.processDNSLookup(r.Context(), w, req)
}

// handleReverseLookup provides legacy PTR lookup endpoint - delegates to normalize.IPToReverseDNS
// @Summary Submit reverse DNS lookup (PTR)
// @Description Enqueue a reverse DNS lookup for an IP address. Automatically converts IP to PTR format.
// @Tags DNS
// @Accept json
// @Produce json
// @Param request body models.ReverseLookupRequest true "Reverse lookup parameters"
// @Success 200 {object} models.TaskResponse "Task accepted and enqueued"
// @Failure 400 {object} models.ErrorResponse "Invalid IP address or missing parameters"
// @Failure 429 {object} models.ErrorResponse "Rate limit exceeded"
// @Failure 503 {object} models.ErrorResponse "No workers available"
// @Router /reverse-lookup [post]
func (s *Server) handleReverseLookup(w http.ResponseWriter, r *http.Request) {
	var oldReq struct {
		ReverseIP             string             `json:"reverse_ip"`
		DNSServers            []models.DNSServer `json:"dns_servers,omitempty"`
		TLSInsecureSkipVerify bool               `json:"tls_insecure_skip_verify,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&oldReq); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	reverseDomain, err := normalize.IPToReverseDNS(oldReq.ReverseIP)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	req := models.DNSLookupRequest{
		Domain:                reverseDomain,
		QType:                 "PTR",
		DNSServers:            oldReq.DNSServers,
		TLSInsecureSkipVerify: oldReq.TLSInsecureSkipVerify,
	}

	s.processDNSLookup(r.Context(), w, req)
}

// processDNSLookup validates request, checks worker availability (Asynq only), enqueues task.
func (s *Server) processDNSLookup(ctx context.Context, w http.ResponseWriter, req models.DNSLookupRequest) {
	if err := req.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check worker availability - only Asynq mode needs this
	if asynqClient, ok := s.tasksClient.(*tasks.Client); ok {
		if !asynqClient.HasActiveWorkers(ctx) {
			respondError(w, http.StatusServiceUnavailable, "no workers available - tasks cannot be processed")
			return
		}
	}

	// Use config servers if none provided
	if len(req.DNSServers) == 0 {
		for _, t := range s.config.GetDNSTargets() {
			req.DNSServers = append(req.DNSServers, models.DNSServer{Target: t.Target, Tags: t.Tags})
		}
	}
	if len(req.DNSServers) == 0 {
		respondError(w, http.StatusBadRequest, "Aucun serveur DNS n'est configuré, veuillez renseigner un serveur et une adresse.")
		return
	}

	// Enforce max servers per request limit (applies to both explicit and config-provided servers)
	maxServers := s.config.GetMaxServersPerRequest()
	if len(req.DNSServers) > maxServers {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("too many DNS servers: %d (maximum allowed: %d). Reduce servers in config or request", len(req.DNSServers), maxServers))
		return
	}

	// Normalize all targets before enqueueing
	for i := range req.DNSServers {
		if norm, err := normalize.Target(req.DNSServers[i].Target); err == nil {
			req.DNSServers[i].Target = norm
		} else {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if s.tasksClient == nil {
		respondError(w, http.StatusInternalServerError, "tasks client not configured")
		return
	}

	id, err := s.tasksClient.EnqueueDNSLookup(ctx, req.Domain, req.QType, req.DNSServers, req.TLSInsecureSkipVerify)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	msg := "DNS lookup enqueued"
	if req.QType == "PTR" {
		msg = "Reverse DNS lookup enqueued"
	}
	respondJSON(w, http.StatusOK, models.TaskResponse{TaskID: id, Message: msg})
}

// handleGetTaskStatus retrieves the status and result of a submitted task
// @Summary Get task status and result
// @Description Retrieve the status and result of a previously submitted DNS lookup task
// @Tags Tasks
// @Produce json
// @Param taskID path string true "Task ID"
// @Success 200 {object} models.TaskStatusResponse "Task found"
// @Failure 404 {object} models.ErrorResponse "Task not found"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /tasks/{taskID} [get]
func (s *Server) handleGetTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if s.tasksClient == nil {
		respondError(w, http.StatusInternalServerError, "tasks client not configured")
		return
	}
	status, err := s.tasksClient.GetTaskStatus(r.Context(), taskID)
	if err != nil {
		if err.Error() == "not found" {
			respondError(w, http.StatusNotFound, "task not found")
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Metrics on-demand: update when client polls results (solves worker metrics collection)
	metrics.APIResultPollsTotal.Inc()
	s.updateMetricsFromTaskResult(taskID, *status)

	respondJSON(w, http.StatusOK, status)
}

// updateMetricsFromTaskResult collects metrics on demand when clients poll results.
// Pragmatic solution: works without worker HTTP endpoints.
func (s *Server) updateMetricsFromTaskResult(_ string, status models.TaskStatusResponse) {
	if status.Status != "SUCCESS" || status.Result == nil {
		return
	}

	for target, detail := range status.Result.Details {
		qtype := detail.QType
		if qtype == "" {
			qtype = "A"
		}

		if detail.CommandStatus == "ok" {
			metrics.DNSLookupTotal.WithLabelValues(target, qtype, "success").Inc()
			metrics.DNSLookupDuration.WithLabelValues(target, qtype).Observe(detail.TimeMs / 1000.0)
		} else {
			metrics.DNSLookupTotal.WithLabelValues(target, qtype, "error").Inc()
			if detail.Error != "" {
				metrics.DNSLookupErrors.WithLabelValues(target, detail.Error).Inc()
			} else {
				metrics.DNSLookupErrors.WithLabelValues(target, "unknown").Inc()
			}
		}
	}
}

// handleHealthCheck returns degraded if Asynq workers unavailable
// @Summary Health check
// @Description Check if the API service is running and workers are available
// @Tags System
// @Produce json
// @Success 200 {object} models.HealthResponse "Service is healthy or degraded"
// @Router /health [get]
// @Router /status [get]
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	health := models.HealthResponse{Status: "ok"}

	if asynqClient, ok := s.tasksClient.(*tasks.Client); ok {
		if !asynqClient.HasActiveWorkers(r.Context()) {
			health.Status = "degraded"
			health.Warning = "no active workers detected"
		}
	}

	if health.Status == "degraded" {
		respondJSON(w, http.StatusServiceUnavailable, health)
		return
	}

	respondJSON(w, http.StatusOK, health)
}

// handleMetrics exposes Prometheus metrics
// @Summary Prometheus metrics
// @Description Expose application metrics in Prometheus format
// @Tags System
// @Produce text/plain
// @Success 200 {string} string "Prometheus metrics"
// @Router /metrics [get]
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

func respondJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// LoadConfigFromEnv provides default config path fallback.
func LoadConfigFromEnv() string {
	p := os.Getenv("CONFIG_PATH")
	if p == "" {
		p = "conf/config.yaml"
	}
	return p
}
