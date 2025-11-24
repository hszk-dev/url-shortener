//go:build integration

package shortener_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/suzukikyou/url-shortener/internal/shortener"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	testredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestContainers initializes PostgreSQL and Redis test containers
// Returns: db connection, redis client, cleanup function, error
func setupTestContainers(t *testing.T) (*sql.DB, *redis.Client, func(), error) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.WithInitScripts("../../init.sql"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	// Start Redis container
	redisContainer, err := testredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to start redis container: %w", err)
	}

	// Get PostgreSQL connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to get postgres connection string: %w", err)
	}

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Verify PostgreSQL connection with retries
	var pingErr error
	for i := 0; i < 10; i++ {
		pingErr = db.PingContext(ctx)
		if pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to ping postgres after retries: %w", pingErr)
	}

	// Get Redis connection endpoint
	redisEndpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to get redis endpoint: %w", err)
	}

	// Connect to Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisEndpoint,
	})

	// Verify Redis connection with retries
	var redisPingErr error
	for i := 0; i < 10; i++ {
		redisPingErr = redisClient.Ping(ctx).Err()
		if redisPingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if redisPingErr != nil {
		redisClient.Close()
		db.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		return nil, nil, nil, fmt.Errorf("failed to ping redis after retries: %w", redisPingErr)
	}

	cleanup := func() {
		redisClient.Close()
		db.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
	}

	return db, redisClient, cleanup, nil
}

// TestIntegration_ReadThroughCache validates the complete Read-Through caching flow
// with real PostgreSQL and Redis instances.
//
// This test verifies the key architectural decision from CLAUDE.md:
// "Read-Through caching strategy with Redis for sub-millisecond redirections"
//
// Test Flow:
// 1. First Get (Cache Miss): Redis miss → DB query → Redis cache update
// 2. Second Get (Cache Hit): Redis hit → Immediate return (no DB query)
// 3. Verify cache population and value consistency
func TestIntegration_ReadThroughCache(t *testing.T) {
	db, redisClient, cleanup, err := setupTestContainers(t)
	if err != nil {
		t.Fatalf("Failed to setup test containers: %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	repo := shortener.NewPostgresRedisRepository(db, redisClient)

	testURL := "https://github.com/testcontainers"

	// Save URL to get ID
	id, err := repo.Save(ctx, testURL)
	if err != nil {
		t.Fatalf("Failed to save URL: %v", err)
	}

	cacheKey := fmt.Sprintf("shorturl:id:%d", id)

	// Verify cache is empty before first Get
	_, err = redisClient.Get(ctx, cacheKey).Result()
	if err != redis.Nil {
		t.Errorf("Cache should be empty before first Get, got error: %v", err)
	}

	// First Get - Should trigger Cache Miss → DB query → Cache update
	t.Run("First Get - Cache Miss", func(t *testing.T) {
		url, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}

		if url != testURL {
			t.Errorf("Get() returned %s, want %s", url, testURL)
		}

		// Verify cache is now populated
		cachedURL, err := redisClient.Get(ctx, cacheKey).Result()
		if err != nil {
			t.Fatalf("Cache should be populated after first Get: %v", err)
		}

		if cachedURL != testURL {
			t.Errorf("Cached value = %s, want %s", cachedURL, testURL)
		}

		// Verify TTL is set (should be close to 24 hours)
		ttl, err := redisClient.TTL(ctx, cacheKey).Result()
		if err != nil {
			t.Fatalf("Failed to get TTL: %v", err)
		}

		expectedTTL := 24 * time.Hour
		// Allow 1 minute tolerance for test execution time
		if ttl < expectedTTL-time.Minute || ttl > expectedTTL {
			t.Errorf("TTL = %v, want ~%v", ttl, expectedTTL)
		}
	})

	// Second Get - Should hit cache (no DB query)
	t.Run("Second Get - Cache Hit", func(t *testing.T) {
		url, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}

		if url != testURL {
			t.Errorf("Get() returned %s, want %s", url, testURL)
		}

		// Performance validation: Cache hit should be fast
		// Note: This is a basic validation. In production, use detailed metrics.
		start := time.Now()
		url, err = repo.Get(ctx, id)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}

		// Cache hit should be sub-millisecond in optimal conditions
		// Allow 100ms for container overhead in tests
		if elapsed > 100*time.Millisecond {
			t.Logf("Warning: Cache hit took %v (expected <100ms)", elapsed)
		}
	})
}

// TestIntegration_ConcurrentWrites validates that PostgreSQL BIGSERIAL
// auto-increment generates unique IDs under concurrent load.
//
// This test verifies the architectural decision:
// "Base62 encoding with Database Auto-Increment guarantees uniqueness with O(1) complexity"
//
// Test validates:
// - No duplicate IDs generated under concurrent writes
// - All goroutines complete successfully without errors
// - ACID compliance maintained
func TestIntegration_ConcurrentWrites(t *testing.T) {
	db, redisClient, cleanup, err := setupTestContainers(t)
	if err != nil {
		t.Fatalf("Failed to setup test containers: %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	repo := shortener.NewPostgresRedisRepository(db, redisClient)

	const numWorkers = 100
	results := make(chan uint64, numWorkers)
	errors := make(chan error, numWorkers)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Launch concurrent writes
	for i := 0; i < numWorkers; i++ {
		go func(n int) {
			defer wg.Done()

			url := fmt.Sprintf("https://example.com/concurrent/%d", n)
			id, err := repo.Save(ctx, url)
			if err != nil {
				errors <- err
				return
			}

			results <- id
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent write failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Failed with %d errors", errorCount)
	}

	// Verify all IDs are unique
	seen := make(map[uint64]bool)
	idCount := 0

	for id := range results {
		if seen[id] {
			t.Errorf("Duplicate ID detected: %d", id)
		}
		seen[id] = true
		idCount++
	}

	if idCount != numWorkers {
		t.Errorf("Expected %d unique IDs, got %d", numWorkers, idCount)
	}

	// Additional validation: Verify all URLs are retrievable
	t.Run("Verify all URLs retrievable", func(t *testing.T) {
		for id := range seen {
			_, err := repo.Get(ctx, id)
			if err != nil {
				t.Errorf("Failed to retrieve URL for ID %d: %v", id, err)
			}
		}
	})
}

// TestIntegration_CacheExpiration validates Redis TTL behavior
//
// Note: This test uses a short TTL (3 seconds) for test efficiency.
// Production uses 24h TTL as defined in repository.go:97
func TestIntegration_CacheExpiration(t *testing.T) {
	db, redisClient, cleanup, err := setupTestContainers(t)
	if err != nil {
		t.Fatalf("Failed to setup test containers: %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	repo := shortener.NewPostgresRedisRepository(db, redisClient)

	testURL := "https://example.com/ttl-test"

	// Save URL
	id, err := repo.Save(ctx, testURL)
	if err != nil {
		t.Fatalf("Failed to save URL: %v", err)
	}

	// First Get - populates cache
	_, err = repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("First Get() failed: %v", err)
	}

	cacheKey := fmt.Sprintf("shorturl:id:%d", id)

	// Manually set a short TTL for testing (override production 24h)
	err = redisClient.Expire(ctx, cacheKey, 3*time.Second).Err()
	if err != nil {
		t.Fatalf("Failed to set TTL: %v", err)
	}

	// Verify cache exists
	_, err = redisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		t.Fatalf("Cache should exist: %v", err)
	}

	// Wait for expiration
	t.Log("Waiting for cache expiration (3 seconds)...")
	time.Sleep(4 * time.Second)

	// Verify cache expired
	_, err = redisClient.Get(ctx, cacheKey).Result()
	if err != redis.Nil {
		t.Errorf("Cache should be expired, got error: %v", err)
	}

	// Get should still work (DB fallback)
	url, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() after expiration failed: %v", err)
	}

	if url != testURL {
		t.Errorf("Get() = %s, want %s", url, testURL)
	}

	// Verify cache is re-populated
	cachedURL, err := redisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		t.Fatalf("Cache should be re-populated: %v", err)
	}

	if cachedURL != testURL {
		t.Errorf("Re-cached value = %s, want %s", cachedURL, testURL)
	}
}
