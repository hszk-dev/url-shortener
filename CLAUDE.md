# Project Context: High-Performance URL Shortener

## 🎯 Project Goal & Scope
This repository demonstrates **production-readiness and architectural patterns** for a scalable URL shortening service.
It focuses on high code quality, robust system design, and clear documentation standard in global tech environments.

### Core Engineering Values
1.  **Scalability:** Designed to handle high traffic and read-heavy workloads (Read-Through Caching).
2.  **Reliability:** Robust error handling, Graceful Shutdown, and ACID compliance.
3.  **Maintainability:** Clean Architecture, comprehensive testing, and idiomatic Go code.
4.  **Documentation:** All communication (Commits, PRs, Issues) follows global English standards.

---

## 🏗 System Architecture & Design Rules

### Tech Stack
* **Language:** Go (Latest Stable)
* **Storage:** PostgreSQL (Persistent), Redis (Cache)
* **Infra:** Docker & Docker Compose
* **Testing:** Standard `testing` package with `go-sqlmock` / `gomock`

### Key Implementation Decisions (Immutable Rules)

#### 1. ID Generation
* **Strategy:** Base62 Encoding with Database Auto-Increment.
* **Why:** Guarantee uniqueness with O(1) complexity. Avoid hash collisions.
* **Constraint:** Do NOT suggest Hashing (MD5/SHA) unless specifically asked for comparison.

#### 2. Caching Pattern
* **Strategy:** **Read-Through**.
* **Flow:** App -> Redis (Hit?) -> Return. If Miss -> DB -> Update Redis -> Return.
* **TTL:** Set appropriate TTL (e.g., 24h) with LRU eviction assumption.

#### 3. HTTP Status Codes
* **Rule:** Use **302 Found** for redirects.
* **Why:** 301 enables browser caching, which kills server-side analytics. We prioritize analytics over bandwidth.

#### 4. Architecture: Clean Architecture
* **Layering:** `Handler` -> `Service` -> `Repository`.
* **Dependency Injection:** Dependencies should be injected via interfaces to enable mocking.

---

## 📝 Git & GitHub Guidelines (Strict)

### Branching Strategy
* `main`: Always deployable. Protected branch (base for all features).
* **Rule:** NEVER commit directly to `main`. ALL changes must go through feature branches and Pull Requests.
* **Branch Naming Convention:**
  - `feat/*`: New features (e.g., `feat/user-auth`)
  - `fix/*`: Bug fixes (e.g., `fix/cache-race-condition`)
  - `test/*`: Test additions/improvements
  - `ci/*`: CI/CD pipeline changes
  - `docs/*`: Documentation updates
  - `refactor/*`: Code refactoring without behavior change
* **Workflow:**
  1. Create feature branch from `main`
  2. Commit changes with proper commit messages
  3. Push branch and create Pull Request
  4. After review/approval, merge to `main` (squash or merge commit)
  5. Delete feature branch after merge

### Commit Messages (The 50/72 Rule)
**MANDATORY Format:**
```text
<type>: <subject (50 chars max, imperative mood)>

<body (wrap at 72 chars, explain WHY not HOW)>
````

  * **Imperative Mood:** "Add feature" (not "Added").
  * **Granularity:** Atomic commits. One logical change per commit.

### Pull Requests (PRs)

  * Must be in **English**.
  * Focus on **Motivation (Why)** and **Verification (How to test)**.

-----

## 🛡 Coding Standards & Best Practices

### Go Specific Rules

1.  **Error Handling:**
      * Never ignore errors (`_`).
      * Use error wrapping: `fmt.Errorf("context: %w", err)` to preserve stack traces/context.
2.  **Testing:**
      * Table-driven tests are preferred.
      * Use interfaces for all external dependencies (DB, Cache) to facilitate mocking.
3.  **Concurrency:**
      * Prefer Channels and WaitGroups over Mutexes where applicable, but choose the simplest solution.
4.  **Linting:**
      * Code must be compliant with `golangci-lint` standard rules.

### Security

  * **Secrets:** Never hardcode credentials. Use environment variables.
  * **Validation:** Sanitize all inputs.

-----

## 🤖 Assistant Instructions (Meta-Rules for Claude)

1.  **Persona:** Act as a Senior Engineer at a top-tier tech company. Be critical of implementation details regarding performance and scalability.
2.  **Explain Trade-offs:** When suggesting a solution, briefly explain *why* it is better than the alternative (e.g., "I chose `map` here for O(1) access, trading memory for speed").
3.  **Convention Enforcement:** If the user provides a commit message that violates the 50/72 rule or uses Japanese, **rewrite it** to follow the standard before proceeding.
4.  **Test First:** Remind the user about tests if they generate implementation code without corresponding tests.
5.  **No "Line Number" References:** When discussing code, refer to function names or logic blocks, as line numbers change.

-----

## 📁 Project Structure Pattern

  * `cmd/`: Main applications.
  * `internal/`: Private application and library code (Service, Repository).
  * `api/`: OpenAPI/Swagger definitions.
  * `deploy/` or root: Docker/Infrastructure configs.

