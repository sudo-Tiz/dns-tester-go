# DNS Tester GO

<p align="center">
  <img src="https://img.shields.io/github/v/release/sudo-Tiz/dns-tester-go?logo=github&sort=semver" alt="release"/>
  <img src="https://img.shields.io/badge/Go-1.23-blue" alt="Go version"/>
  <img src="https://img.shields.io/docker/pulls/Sudo-Tiz/dnstestergo.svg" alt="docker"/>
  <img src="https://img.shields.io/github/actions/workflow/status/sudo-Tiz/dns-tester-go/build.yml?branch=main" alt="build"/>
  <img src="https://img.shields.io/badge/semantic--release-conventional-e10079?logo=semantic-release" alt="semantic-release"/>
  <img src="https://img.shields.io/badge/status-beta-yellow" alt="beta"/>
</p>

High-performance DNS testing tool supporting UDP, TCP, DoT, DoH, and DoQ protocols.

> ⚠️ **Beta Status**: This project is under active development. Bugs and breaking changes may occur. Contributions and feedback are highly encouraged!

📖 **[Documentation](https://sudo-Tiz.github.io/dns-tester-go)** | 
🚀 **[Quick Start](https://sudo-Tiz.github.io/dns-tester-go/docs/quickstart)** |
🛠️ **[Installation](https://sudo-Tiz.github.io/dns-tester-go/docs/installation)** |
💻 **[CLI Guide](https://sudo-Tiz.github.io/dns-tester-go/docs/cli)** |
📡 **[API Reference](https://sudo-Tiz.github.io/dns-tester-go/docs/api)** |
⚡ **Benchmarks** *(coming soon)*

## 🚀 Quick Start

```bash
# Docker (recommended)
docker compose up -d

# CLI
go install github.com/sudo-Tiz/dns-tester-go/cmd/dnstestergo@latest
dnstestergo query example.com quic://dns.adguard-dns.com:853

# Using Makefile
make build        # Build binaries
make test         # Run tests
make docker-dev   # Start Docker stack
```

For detailed installation, usage, configuration, and API documentation, see **[the documentation](https://sudo-Tiz.github.io/dns-tester-go)**.

> 🤖 Documentation is partially generated using AI agents and may contain errors.

## 🤝 Contributing

We welcome contributions! This is a beta project and your help is valuable:

- 🐛 Report bugs via [GitHub Issues](https://github.com/sudo-Tiz/dns-tester-go/issues)
- 💡 Suggest features or improvements
- 📖 Improve documentation
- 🔧 Submit pull requests

See **[Contributing Guide](https://sudo-Tiz.github.io/dns-tester-go/docs/contributing)**.

## 📋 Roadmap

See **[TODO.md](TODO.md)** for upcoming features and improvements.

## 📝 License

See [LICENSE](LICENSE) file for details.

## 🙏 Credits

This project is a complete Go rewrite of **[DNS-Tester](https://github.com/dmachard/DNS-Tester)**.

Special thanks to [@dmachard](https://github.com/dmachard) for the original Python implementation and design inspiration.

---

**[Documentation](https://sudo-Tiz.github.io/dns-tester-go)** |
**[Issues](https://github.com/sudo-Tiz/dns-tester-go/issues)** |
**[Releases](https://github.com/sudo-Tiz/dns-tester-go/releases)** |
**[TODO](TODO.md)**
