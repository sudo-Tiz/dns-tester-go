# Contributing

## Development Setup

```bash
git clone https://github.com/sudo-tiz/dns-tester-go.git
cd dns-tester-go

# Install dependencies
go mod download

# Install prek hooks (REQUIRED)
make install-prek

# Alternative: use pre-commit if you prefer (fully compatible)
# pip install pre-commit && pre-commit install

# Run tests
make test

# Build
make build
```

**Prek hooks will automatically:**
- Format code (gofmt, goimports)
- Run linters (golangci-lint)
- Validate YAML files
- Check conventional commit messages
- Generate Swagger docs

> **Note:** We use [prek](https://prek.j178.dev/) (faster Rust alternative to pre-commit), but the standard `pre-commit` tool also works with our `.pre-commit-config.yaml`.

## Project Structure

```
dns-tester-go/
├── cmd/              # Entry points (api, worker, cli)
├── internal/         # Core logic
│   ├── api/         # HTTP handlers
│   ├── cli/         # CLI commands
│   ├── resolver/    # DNS resolution (dnsproxy wrapper)
│   ├── tasks/       # Asynq task handlers
│   ├── models/      # Data models
│   └── config/      # Configuration
├── docs/            # Documentation (Markdown)
└── website/         # Docusaurus site
```

## Code Style

**Automated by prek hooks:**
- Go formatting (`gofmt`, `goimports`)
- Linting (`golangci-lint`)
- YAML validation
- Max line length: 120 chars

**Commit messages (enforced by prek):**
```
type(scope): subject

body (optional)
```

**Types:** `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`, `perf`

**Example:**
```
feat(api): add DoQ protocol support

Implements DNS-over-QUIC using AdGuard dnsproxy.
Closes #123
```

**Manual checks:**
```bash
# Run all prek hooks manually
make prek

# Run specific linters
make lint
```

## Testing

**Run all tests:**
```bash
make test
```


**E2E tests (requires Docker):**
```bash
make test-e2e-docker
```

## Pull Request Process

1. **Fork & branch:**
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make changes:**
   - Write tests
   - Update docs if needed
   - Follow code style

3. **Commit:**
   ```bash
   git commit -m "feat(api): add new endpoint"
   ```

4. **Push & PR:**
   ```bash
   git push origin feat/my-feature
   ```
   Open PR with description

5. **CI checks:**
   - Tests pass
   - Linting passes
   - No decrease in coverage

## Documentation

**Edit docs:**
```bash
cd docs/
# Edit .md files
```

**Preview website:**
```bash
cd website/
npm install
npm start
```

**Sync docs to website:**
```bash
cd website/
./sync-docs.sh
```

## Release Process

**Maintainers only:**

Releases are automated via semantic-release based on conventional commits:

1. Merge PR to `main` with conventional commit message
2. GitHub Actions automatically:
   - Analyzes commits to determine version bump
   - Creates git tag (e.g., `v1.2.3`)
   - Builds multi-platform binaries
   - Publishes Docker images to ghcr.io
   - Creates GitHub Release with artifacts

**Manual release (emergency):**
```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

## Getting Help

| Channel | Link |
|---------|------|
| Issues | [GitHub Issues](https://github.com/sudo-tiz/dns-tester-go/issues) |
| Discussions | [GitHub Discussions](https://github.com/sudo-tiz/dns-tester-go/discussions) |
| Docs | [Documentation](https://sudo-tiz.github.io/dns-tester-go) |

## Code of Conduct

Be respectful and constructive. Follow [Go Community Code of Conduct](https://go.dev/conduct).
