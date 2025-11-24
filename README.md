# High-Performance URL Shortener Service

A scalable, production-ready URL shortening service built with Go, PostgreSQL, and Redis. Designed to handle high read throughput with low latency redirection, demonstrating modern system design principles.

## 🚀 Demo

![Demo GIF](https://via.placeholder.com/800x400?text=Demo+GIF+Placeholder)
*(Replace this with an actual GIF of the application in action)*

**Live Demo:** [https://your-demo-url.com](https://your-demo-url.com)

## 🛠 Tech Stack

- **Language:** Go (Golang)
- **Database:** PostgreSQL (Persistent Storage)
- **Cache:** Redis (In-memory Cache)
- **Infrastructure:** Docker, Docker Compose
- **Documentation:** Swagger / OpenAPI

## ✨ Key Features

- **High Performance:** Utilizes Base62 encoding for efficient, collision-free ID generation.
- **Low Latency:** Implements a Read-Through caching strategy with Redis for sub-millisecond redirections.
- **Scalable Architecture:** Containerized with Docker for easy deployment and horizontal scaling.
- **Concurrency Safe:** Handles concurrent requests for custom aliases using database constraints.
- **Analytics Ready:** Uses HTTP 302 redirects to enable future click tracking and analytics.

## 🏗 Architecture / Design Decisions

This project was designed with a focus on **scalability**, **performance**, and **reliability**. Below are the key architectural decisions and the rationale behind them.

### 1. ID Generation: Base62 Encoding vs. Hashing
I chose **Base62 encoding** over traditional hashing algorithms (like MD5 or SHA-256) for generating short URLs.

- **The Problem with Hashing:** Hashing algorithms produce long strings (e.g., MD5 produces 32 hex characters). Truncating them to a desired length (e.g., 6 characters) introduces a high probability of **collisions**. Handling these collisions requires expensive database lookups to check for existence, which degrades write performance.
- **The Base62 Solution:** By using an auto-incrementing integer ID (or a distributed ID generator like Snowflake) and converting it to Base62 (`[a-z, A-Z, 0-9]`), we achieve a **bijective mapping**. This guarantees uniqueness mathematically, eliminating the need for collision checks entirely. This results in **O(1)** time complexity for insertion and ensures the shortest possible URL length for a given number of records.

### 2. Caching Strategy: Read-Through Pattern
Given that URL shorteners are typically **Read-Heavy** systems (often exceeding a 100:1 read-to-write ratio), minimizing database load is critical.

- **Implementation:** I implemented a **Read-Through** caching strategy using **Redis**.
    1.  **Cache Hit:** The service first checks Redis. If the key exists, it returns the long URL immediately (sub-millisecond latency).
    2.  **Cache Miss:** If not found in Redis, the service queries PostgreSQL.
    3.  **Cache Update:** The result from the DB is then stored in Redis with an LRU (Least Recently Used) eviction policy to keep frequently accessed URLs hot.
- **Benefit:** This significantly reduces the load on the primary database and ensures the system can handle traffic spikes efficiently.

### 3. Database Choice: Relational (PostgreSQL) vs. NoSQL
While NoSQL databases (like DynamoDB) offer high scalability, I selected **PostgreSQL** for this implementation.

- **Reasoning:** PostgreSQL provides **ACID compliance**, which is crucial for ensuring data integrity, especially when handling custom aliases where uniqueness must be strictly enforced.
- **Future Scaling:** For a massive scale (billions of URLs), I would consider sharding the PostgreSQL database or migrating to a wide-column store like Cassandra, but for the current scope, PostgreSQL handles the projected load (millions of records) with excellent performance and reliability.

### 4. HTTP Status Code: 302 Found vs. 301 Moved Permanently
I opted for **HTTP 302 (Found)** for redirections.

- **Trade-off:**
    - **301:** Browsers cache the redirect permanently. This reduces server load but makes it impossible to track click analytics after the first visit.
    - **302:** Browsers hit the server for every request.
- **Decision:** Since analytics (tracking click counts, referrers, etc.) are a core business value for URL shorteners, using 302 allows the server to capture every interaction, prioritizing data value over minimal bandwidth savings.

## ⚡ Performance Benchmarks

Benchmarked using **k6** on a local Docker environment.

- **Throughput:** ~500 requests/sec (limited by local environment)
- **Latency:** <50ms (99th percentile)
- **Tool:** k6

*(Note: Real-world performance would be significantly higher on production hardware with proper tuning.)*

## 🧠 Interview Preparation

I have prepared a comprehensive guide for System Design Interview questions related to this project.
👉 **[Read the Interview Prep Guide](INTERVIEW_PREP.md)**


## 📦 Getting Started

### Prerequisites
- Docker & Docker Compose installed

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/url-shortener.git
   cd url-shortener
   ```

2. **Start the services**
   ```bash
   docker-compose up -d
   ```

3. **Verify the service**
   The server will start on port `8080`.
   ```bash
   curl -X POST http://localhost:8080/api/shorten -d '{"url": "https://www.google.com"}'
   ```

## 🧪 Running Tests

```bash
go test ./...
```
