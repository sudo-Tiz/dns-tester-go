# Architecture

High-level system design and data flow.

---

## 🏗️ System Overview

```
Client → API Server → Asynq (Redis) → Worker Pool → DNS Servers
                         ↓                ↓
                      Results ← ─ ─ ─ ─ ─ ┘
```

---

## 📊 Components

```mermaid
graph TD
    Client([Client])
    API[API Server<br/>Port 5000]
    Redis[(Redis<br/>Task Queue)]
    Worker[Workers<br/>1-N instances]
    DNS[DNS Servers<br/>UDP/DoT/DoH/DoQ]
    Metrics[metrics<br/>Prometheus]
    
    Client -->|POST /dns-lookup| API
    API -->|Enqueue task| Redis
    Redis -->|Pull task| Worker
    Worker -->|DNS query| DNS
    DNS -->|Response| Worker
    Worker -->|Store result| Redis
    Client -->|GET /tasks/id| API
    API -->|Fetch result| Redis
    API -.->|Expose| Metrics
    Worker -.->|Expose| Metrics
    
    style Redis fill:#f96,stroke:#333,stroke-width:2px
    style DNS fill:#9cf,stroke:#333,stroke-width:2px
```

---

## 🔄 Request Flow


```
┌─────────┐  1. POST /dns-lookup         ┌─────────────┐
│ Client  │─────────────────────────────>│  API Server │
└─────────┘                              │  chi router │
                                         └──────┬──────┘
                                                │ 2. Validate
                                                │ 3. Rate limit
                                                ▼
                                         ┌─────────────┐
                                         │   Asynq     │
                                         │   Enqueue   │
                                         └──────┬──────┘
                                                │ 4. Store task
                                                ▼
┌─────────┐  202 {task_id}               ┌──────────────┐
│ Client  │<─────────────────────────────│    Redis     │
└────┬────┘                              └──────┬───────┘
     │                                          │ 5. Dequeue
     │ 6. Poll GET /tasks/{id}                  ▼
     │                                   ┌──────────────┐
     │                                   │  Worker Pool │
     │                                   │  (dnsproxy)  │
     │                                   └──────┬───────┘
     │                                          │ 7. Query DNS
     │                                          ▼
     │                                   ┌──────────────┐
     │                                   │  DNS Servers │
     │                                   │ UDP/TCP/TLS/ │
     │                                   │ HTTPS/QUIC   │
     │                                   └──────┬───────┘
     │                                          │ 8. Response
     │                                          ▼
     │                                   ┌──────────────┐
     │                                   │    Redis     │
     │                                   │ Store result │
     │                                   └──────┬───────┘
     │                                          │
     ▼                                          │
┌───────────────────────────────────────────────┘
│  9. Fetch result
▼
{task_status: SUCCESS, task_result: {...}}
```

---

## 🧩 Components

| Component | Technology | Responsibility | Scalability |
|-----------|-----------|----------------|-------------|
| API Server | chi + Tollbooth | HTTP routing, rate limiting | Stateless, horizontal |
| Task Queue | Asynq + Redis | Task persistence, distribution | Redis cluster |
| Worker Pool | Go + dnsproxy | DNS query execution | Configurable concurrency |
| Storage | Redis | Task state, results | Redis cluster |
| Metrics | Prometheus | Observability | Pull-based |

---

## 🔁 Task Lifecycle

```mermaid
stateDiagram-v2
    [*] --> PENDING: Enqueue
    PENDING --> ACTIVE: Worker dequeue
    ACTIVE --> SUCCESS: Query OK
    ACTIVE --> FAILURE: Query error
    SUCCESS --> [*]: Cleanup (10min)
    FAILURE --> [*]: Cleanup (10min)
```

---

## 🚀 Deployment Architectures

### Single Instance (Development)
```
┌──────────────────────────────────┐
│       Docker Compose             │
│                                  │
│  ┌──────┐  ┌────────┐  ┌──────┐  │
│  │ API  │→ │ Redis  │← │Worker│  │
│  └──────┘  └────────┘  └──────┘  │
└──────────────────────────────────┘
```

### Multi-Instance (Production)

```
      ┌──────────┐
      │  LB/Nginx│
      └─────┬────┘
            │
    ┌───────┴───────┐
    ▼               ▼
┌─────────┐    ┌──────────┐
│  API-1  │    │  API-2   │
└───┬─────┘    └────┬─────┘
    │               │
    └───────┬───────┘
            ▼
     ┌─────────────┐
     │Redis Cluster│
     └──────┬──────┘
            │
  ┌─────────┼────────┐
  ▼         ▼        ▼
┌──────┐┌──────┐┌──────┐
│Work-1││Work-2││Work-N│
└──────┘└──────┘└──────┘
```

### Kubernetes (Scalable)

```mermaid
graph TB
    Ingress[Ingress Controller]
    API1[API Pod 1]
    APIN[API Pod N]
    Redis[Redis StatefulSet]
    Worker1[Worker Pod 1]
    WorkerN[Worker Pod N]
    Prom[Prometheus]
    
    Ingress --> API1
    Ingress --> APIN
    API1 --> Redis
    APIN --> Redis
    Redis --> Worker1
    Redis --> WorkerN
    API1 -.->|/metrics| Prom
    APIN -.->|/metrics| Prom
    Worker1 -.->|metrics| Prom
    WorkerN -.->|metrics| Prom
```

---

## 🔧 Protocol Stack

| Layer | Component | Implementation |
|-------|-----------|----------------|
| API | HTTP Router | chi |
| Queue | Task Management | Asynq |
| Worker | Concurrency | Go goroutines |
| DNS | Multi-Protocol | AdGuard dnsproxy |
| Transport | UDP/TCP/TLS/HTTPS/QUIC | miekg/dns, crypto/tls, net/http, quic-go |

---

## 📈 Scaling

| Component | Horizontal | Vertical | Limit |
|-----------|-----------|----------|-------|
| API | ✅ Stateless | Low CPU/Memory | Unlimited |
| Worker | ✅ Task-based | Moderate CPU | DNS rate limits |
| Redis | ⚠️ Cluster needed | High memory | 10k req/s (single) |

**Concurrency:** `Total = Workers × MAX_WORKERS`

---

## ⚡ Performance

> 🔬 **Benchmarks in progress** - Comprehensive performance comparison coming soon.

---

## 🔐 Security Layers

```
Internet → TLS (Reverse Proxy) → Auth (optional) → API → Internal Network (Redis/Workers)
```

1. TLS termination at reverse proxy
2. Rate limiting (proxy + API)
3. Optional authentication
4. Network isolation for Redis
5. Input validation

---

## ❌ Error Handling

| Error | HTTP Code | Behavior |
|-------|-----------|----------|
| Invalid request | 400 | Immediate rejection |
| Rate limit | 429 | Backoff required |
| No workers | 503 | Retry later |
| DNS timeout | 200 | Per-server error in result |

**Philosophy:** API never fails for DNS errors - each server independent, partial success allowed.

---

## 📊 Monitoring

```
API/Workers → /metrics → Prometheus → Grafana → Alertmanager
```

See [Monitoring Guide](06-monitoring.md) for full details.

---

## 📚 See Also

- [API Reference](03-api.md) - REST API documentation
- [Configuration](05-configuration.md) - Config options
- [Monitoring](06-monitoring.md) - Metrics and alerting
