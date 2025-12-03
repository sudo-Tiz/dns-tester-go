# Installation

Choose your installation method.

---

## 📦 Installation Methods

| Method | Command |
|--------|---------|
| **Docker** | `docker compose --profile prod up -d` |
| **Go Install** | `go install github.com/sudo-Tiz/dns-tester-go/cmd/dnstestergo@latest` |
| **Binary** | [Download from Releases](https://github.com/sudo-Tiz/dns-tester-go/releases) |
| **Source** | `make build` |

---

## 🐳 Docker (Recommended)

```bash
git clone https://github.com/sudo-Tiz/dns-tester-go.git
cd dns-tester-go

# Production setup (API + Worker + Redis)
cp conf/config.example.yaml conf/config.yaml  # Create config file
# Edit conf/config.yaml with your DNS servers
docker compose --profile prod up -d

# OR Development (all-in-one, no Redis, uses config.example.yaml)
docker compose --profile dev up -d
```

---

## 🔧 Go Install

```bash
go install github.com/sudo-Tiz/dns-tester-go/cmd/dnstestergo@latest
```

---

## 🛠️ Build from Source

```bash
git clone https://github.com/sudo-Tiz/dns-tester-go.git
cd dns-tester-go
make build
```

**Binaries**: `bin/dnstestergo`, `bin/dnstestergo-query`, `bin/dnstestergo-server`, `bin/dnstestergo-worker`

---

## 📥 Download Pre-built Binaries

[View all releases](https://github.com/sudo-Tiz/dns-tester-go/releases)

**Quick Install:**
```bash
# Set your platform (linux/darwin/windows) and arch (amd64/arm64)
VERSION=v1.0.0  # Replace with latest version
OS=linux        # or darwin (macOS), windows
ARCH=amd64      # or arm64

# Download and install
curl -L "https://github.com/sudo-Tiz/dns-tester-go/releases/download/${VERSION}/dnstestergo-${OS}-${ARCH}" -o dnstestergo && \
chmod +x dnstestergo && \
sudo mv dnstestergo /usr/local/bin/
```

**Available binaries:**
- `dnstestergo-{os}-{arch}` - All-in-one (recommended)
- `dnstestergo-server-{os}-{arch}` - API server only
- `dnstestergo-worker-{os}-{arch}` - Worker only
- `dnstestergo-query-{os}-{arch}` - CLI query tool

**Supported:** `linux/darwin/windows` × `amd64/arm64`

**Windows:** Download `.exe` from releases and add to PATH.

---

## 🐛 Troubleshooting

[See Troubleshooting](07-troubleshooting.md)

---

## 📚 Next

- [Quick Start](01-quickstart.md) - Test the system
- [CLI Guide](04-cli.md) - All commands and flags
- [Configuration](05-configuration.md) - Configure DNS servers
