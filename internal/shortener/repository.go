package shortener

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrNotFound = errors.New("url not found")
)

type Repository interface {
	Save(ctx context.Context, originalURL string) (uint64, error)
	Get(ctx context.Context, id uint64) (string, error)
	Close() error
}

type PostgresRedisRepository struct {
	db     *sql.DB
	redis  *redis.Client
	logger *log.Logger
}

func NewPostgresRedisRepository(db *sql.DB, redisClient *redis.Client) *PostgresRedisRepository {
	return &PostgresRedisRepository{
		db:     db,
		redis:  redisClient,
		logger: log.New(os.Stderr, "[repository] ", log.LstdFlags),
	}
}

func (r *PostgresRedisRepository) Save(ctx context.Context, originalURL string) (uint64, error) {
	// Simple INSERT returning ID.
	// In a real distributed system, we might use a dedicated ID generator (Snowflake).
	// For this scope, Postgres SERIAL/BIGSERIAL is sufficient and robust.
	var id uint64
	query := `INSERT INTO urls (original_url) VALUES ($1) RETURNING id`
	err := r.db.QueryRowContext(ctx, query, originalURL).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to save url: %w", err)
	}
	return id, nil
}

// Get retrieves the original URL for a given ID using Read-Through caching.
//
// The caller should set an appropriate timeout on ctx. Recommended: 3-5 seconds.
// This allows time for Redis lookup (~100ms) and DB query (~3s) with buffer for retries.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	url, err := repo.Get(ctx, id)
//
// Performance: Redis cache hit returns in <1ms. Cache miss requires DB query (~10-50ms).
//
// Future Improvement: Consider using golang.org/x/sync/singleflight to prevent
// cache stampede (multiple concurrent requests for the same expired cache entry
// all hitting the database simultaneously).
func (r *PostgresRedisRepository) Get(ctx context.Context, id uint64) (string, error) {
	cacheKey := fmt.Sprintf("shorturl:id:%d", id)

	// 1. Check Redis (Read-Through Cache) - skip if redis is nil (e.g., in tests)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			return val, nil // Cache Hit
		}
		if err != redis.Nil {
			// Log error but proceed to DB (graceful degradation)
			r.logger.Printf("redis get failed for key=%s: %v", cacheKey, err)
		}
	}

	// 2. Check Database (Cache Miss)
	var originalURL string
	query := `SELECT original_url FROM urls WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&originalURL)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get url for id %d: %w", id, err)
	}

	// 3. Update Redis - skip if redis is nil
	if r.redis != nil {
		// Set with expiration (24 hours) to manage memory with LRU eviction
		err = r.redis.Set(ctx, cacheKey, originalURL, 24*time.Hour).Err()
		if err != nil {
			r.logger.Printf("redis set failed for key=%s: %v", cacheKey, err)
		}
	}

	return originalURL, nil
}

// Close closes both database and Redis connections.
// Returns an error if either close operation fails.
func (r *PostgresRedisRepository) Close() error {
	var dbErr, redisErr error

	if r.db != nil {
		dbErr = r.db.Close()
	}

	if r.redis != nil {
		redisErr = r.redis.Close()
	}

	// Return first error encountered, or combine errors if both fail
	if dbErr != nil && redisErr != nil {
		return fmt.Errorf("failed to close connections: db=%v, redis=%v", dbErr, redisErr)
	}
	if dbErr != nil {
		return fmt.Errorf("failed to close database: %w", dbErr)
	}
	if redisErr != nil {
		return fmt.Errorf("failed to close redis: %w", redisErr)
	}

	return nil
}
