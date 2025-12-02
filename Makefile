.PHONY: build build-all clean deps docker-build-all docker-build-dev \
    docker-build-query docker-build-server docker-build-test docker-clean \
    docker-dev docker-down docker-logs docker-prod docker-scale-workers \
    install install-prek lint prek swagger test test-e2e \
    test-e2e-docker test-verbose


# Default target
help:
	@echo "DNS Tester GO - Go Edition"
	@echo ""
	@echo "=== Build ==="
	@echo "  build          Build dnstestergo binary"
	@echo "  build-all      Build all binaries (server, worker, query, cli)"
	@echo "  install        Install dnstestergo to /usr/local/bin"
	@echo ""
	@echo "=== Test ==="
	@echo "  test           Run unit tests"
	@echo "  test-verbose   Run tests with verbose output"
	@echo "  test-e2e       Run E2E tests (requires Docker stack)"
	@echo "  lint           Run Go linters (vet, fmt, golangci-lint)"
	@echo "  prek           Run all prek hooks"
	@echo ""
	@echo "=== Docker ==="
	@echo "  docker-build-all     Build all Docker images"
	@echo "  docker-dev           Start dev environment (memory mode)"
	@echo "  docker-prod          Start production environment"
	@echo "  docker-scale-workers Scale workers (N=5)"
	@echo "  docker-logs          Show logs (SERVICE=server)"
	@echo "  docker-down          Stop all containers"
	@echo "  docker-clean         Remove containers, volumes, images"
	@echo ""
	@echo "=== Other ==="
	@echo "  deps                 Download and tidy dependencies"
	@echo "  swagger              Generate Swagger docs (run after API changes)"
	@echo "  install-prek         Install prek hooks"
	@echo "  clean                Clean build artifacts"


# ============================================================================
# Build Targets
# ============================================================================

# Build all binaries (multi-binary)
build-all: build-dnstestergo build-worker build-server build-query

# Build dnstestergo (monolith/all-in-one)
build build-dnstestergo:
	@go build -o bin/dnstestergo ./cmd/dnstestergo

# Build worker binary
build-worker:
	@go build -o bin/dnstestergo-worker ./cmd/worker

# Build server binary
build-server:
	@go build -o bin/dnstestergo-server ./cmd/api

# Build query binary
build-query:
	@go build -o bin/dnstestergo-query ./cmd/query

# Install dnstestergo to /usr/local/bin
install: build
	@sudo install -m 755 bin/dnstestergo /usr/local/bin/dnstestergo
	@echo "✅ Installed to /usr/local/bin/dnstestergo"

# ============================================================================
# Test Targets
# ============================================================================

# Run all tests
test:
	@go test ./...

# Run tests with verbose output
test-verbose:
	@go test -v ./...

# Run E2E tests (requires API server, Redis, Worker to be running)
test-e2e:
	@echo "Running E2E tests (requires Docker stack: make docker-prod)"
	@RUN_E2E_TESTS=1 go test -v -tags=e2e ./internal/api/...

# Run E2E tests with Docker (starts stack, runs tests, stops stack)
test-e2e-docker:
	@docker compose --profile prod up -d
	@echo "Waiting for services..."
	@sleep 5
	@RUN_E2E_TESTS=1 API_BASE_URL=http://localhost:5000 go test -v -tags=e2e ./internal/api/... || (docker compose --profile prod down && exit 1)
	@docker compose --profile prod down
	@echo "✅ E2E tests completed"

# Install prek hooks
install-prek:
	@which prek > /dev/null || cargo install --git https://github.com/j178/prek
	@prek install
	@prek install --hook-type commit-msg
	@echo "✅ Prek hooks installed in .git/hooks/"

# Run Go linters (format, vet, and check)
lint:
	@echo "Running Go linters..."
	@echo "→ Formatting code..."
	@go fmt ./...
	@echo "→ Running go vet..."
	@go vet ./...
	@echo "→ Checking formatting..."
	@test -z "$$(gofmt -l .)" || (echo "❌ Files need formatting:" && gofmt -l . && exit 1)
	@echo "→ Running golangci-lint..."
	@if which golangci-lint > /dev/null; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "⚠️  golangci-lint not found (optional). Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi
	@echo "✅ All linters passed"

# Run all prek checks (Go, YAML, commits, etc.)
prek:
	@prek run --all-files

# ============================================================================
# Docker Build Targets
# ============================================================================

# Build all Docker images
docker-build-all: docker-build-server docker-build-worker docker-build-query docker-build-dev

# Build server image
docker-build-server:
	@docker build --target server -t dnstestergo:server .

# Build worker image
docker-build-worker:
	@docker build --target worker -t dnstestergo:worker .

# Build query image
docker-build-query:
	@docker build --target query -t dnstestergo:query .

# Build dev image
docker-build-dev:
	@docker build --target dev -t dnstestergo:dev .

# ============================================================================
# Docker Run Targets
# ============================================================================

# Start development environment (memory mode, single container)
docker-dev:
	@echo "Starting dev environment → http://localhost:5000"
	@docker compose --profile dev up --build

# Start Redis only (useful for local development)
docker-redis:
	@docker compose --profile prod up redis -d
	@echo "✅ Redis started → redis://localhost:6379/0"

# Start production environment (server + workers + redis)
docker-prod:
	@docker compose --profile prod up --build -d
	@echo "✅ Production stack started"
	@echo "   API: http://localhost:5000"
	@echo "   Redis: localhost:6379"

# ============================================================================
# Docker Management Targets
# ============================================================================

# Scale workers (usage: make docker-scale-workers N=5)
docker-scale-workers:
	@if [ -z "$(N)" ]; then \
		echo "Usage: make docker-scale-workers N=5"; \
		exit 1; \
	fi
	@docker compose --profile prod up --scale dnstestergo-worker=$(N) -d
	@echo "✅ Workers scaled to $(N) replicas"

# Show logs (usage: make docker-logs SERVICE=server)
docker-logs:
	@if [ -z "$(SERVICE)" ]; then \
		docker compose --profile prod logs -f; \
	else \
		docker compose --profile prod logs -f $(SERVICE); \
	fi

# Stop all containers
docker-down:
	@docker compose --profile dev down
	@docker compose --profile prod down
	@docker compose --profile test down
	@echo "✅ All containers stopped"

# Clean all Docker resources (containers, volumes, images)
docker-clean: docker-down
	@docker compose --profile dev down -v
	@docker compose --profile prod down -v
	@docker compose --profile test down -v
	@docker rmi -f dnstestergo:server dnstestergo:worker dnstestergo:query dnstestergo:dev 2>/dev/null || true
	@echo "✅ Docker resources cleaned"

# ============================================================================
# Utility Targets
# ============================================================================

# Clean build artifacts
clean:
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean
	@echo "✅ Build artifacts cleaned"

# Install dependencies
deps:
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies installed"

# ============================================================================
# Swagger/OpenAPI Documentation
# ============================================================================

# Generate Swagger documentation (run manually after API changes)
swagger:
	@echo "Generating Swagger docs..."
	@go run github.com/swaggo/swag/cmd/swag@latest init \
		-g cmd/api/main.go -o internal/api/docs \
		--parseDependency --parseInternal
	@cp internal/api/docs/swagger.yaml docs/openapi.yaml
	@echo "✅ Swagger docs generated:"
	@echo "   - internal/api/docs/swagger.{yaml,json} (for code imports)"
	@echo "   - docs/openapi.yaml (for GitHub/documentation)"
	@echo "   - docs.go (auto-generated, gitignored)"
