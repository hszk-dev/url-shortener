
# High-Performance URL Shortener Service

![CI](https://github.com/hszk-dev/url-shortener/actions/workflows/ci.yml/badge.svg)

A URL shortening service built with **Go**, **PostgreSQL**, and **Redis**.

## 🚀 Key Features

| Feature | Description |
| :--- | :--- |
| **Efficient ID Generation** | **Base62 encoding** ensures mathematical uniqueness with O(1) complexity. |
| **Low Latency** | **Read-Through caching** (Redis) minimizes DB load for read-heavy traffic. |
| **Reliability** | **PostgreSQL** ensures ACID compliance for data integrity. |
| **Observability** | **HTTP 302** redirects allow for future analytics and click tracking. |

## 🏗 Architecture Highlights

### Why Base62?
Unlike hashing (MD5/SHA), Base62 encoding creates a **bijective mapping** between a unique integer ID and a short string. This eliminates the risk of collisions and removes the overhead of collision checking.

### Caching Strategy
Implements a **Read-Through** pattern:
1. Check Redis (Cache Hit) -> Return immediately.
2. Check DB (Cache Miss) -> Store in Redis -> Return.
This design handles the "Read-Heavy" nature of URL shorteners (often 100:1 read/write ratio).

## ⚡ Performance
Benchmarked with k6 (100 concurrent users):
> **490 req/sec** with **<4ms p99 latency** on local Docker environment.

## 📦 Getting Started

### Installation
```bash
git clone https://github.com/hszk-dev/url-shortener.git
docker-compose up -d
```

### API Documentation

Swagger UI is available at: `http://localhost:8080/docs/`

### Running Tests

| Test Type | Command | Prerequisites |
|:---|:---|:---|
| Unit | `go test ./...` | None |
| Integration | `go test -tags=integration -v ./internal/shortener/` | Docker |
| E2E | `go test -tags=e2e -v ./tests/` | `docker-compose up -d` |
