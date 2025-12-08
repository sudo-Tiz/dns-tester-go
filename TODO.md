# TODO & Roadmap

## 🎯 High Priority

 - [ ] **FIX Worker Redis Implementation** 🛠️
   - Redo the worker implementation with Redis (current code is not optimized at all)

- [ ] **Performance Benchmarks** 📊
  - Comprehensive benchmark suite (protocols, concurrency, throughput)
  - Comparison with Python implementation
  - Latency analysis (p50, p95, p99)
  - Resource consumption metrics
  - Published results in docs

- [ ] **Helm Charts** 🚢
  - Package for Kubernetes deployment
  - ConfigMaps for configuration
  - HPA (Horizontal Pod Autoscaling)
  - Ingress templates

- [ ] **Grafana Dashboard Templates** 📈
  - Pre-built dashboards for common use cases
  - Server comparison views
  - Protocol performance analysis
  - Alert visualization

## 🔧 Features

- [ ] **Enhanced Protocols**
  - DNSCrypt support
  - Oblivious DoH (ODoH)

- [ ] **CLI Enhancements**
  - Interactive mode
  - Output formats (CSV, JSON, Table)
  - Batch query support

- [ ] **API Improvements**
  - WebSocket for real-time updates
  - Batch operations
  - Authentication & authorization

## ✅ Completed

- [x] **Prometheus Configuration** (conf/prometheus.yml)
- [x] Metrics endpoints (API + Worker)
- [x] Multi-protocol support (UDP, TCP, DoT, DoH, DoQ)
- [x] Distributed workers with Redis
- [x] Docker deployment
- [x] CI/CD pipeline
- [x] Documentation website

## 💡 Ideas

Have an idea? [Open an issue](https://github.com/sudo-Tiz/dns-tester-go/issues) with the `enhancement` label!

## 🤝 Contributing

Want to contribute? Check the **[Contributing Guide](CONTRIBUTING.md)** and pick something from this list!

---

**Last updated:** December 2, 2025
