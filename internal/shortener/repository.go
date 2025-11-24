package shortener

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	db    *sql.DB
	redis *redis.Client
}

func NewPostgresRedisRepository(db *sql.DB, redisClient *redis.Client) *PostgresRedisRepository {
	return &PostgresRedisRepository{
		db:    db,
		redis: redisClient,
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

func (r *PostgresRedisRepository) Get(ctx context.Context, id uint64) (string, error) {
	cacheKey := fmt.Sprintf("url:%d", id)

	// 1. Check Redis (Read-Through Cache) - skip if redis is nil (e.g., in tests)
	if r.redis != nil {
		val, err := r.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			return val, nil // Cache Hit
		}
		if err != redis.Nil {
			// Log error but proceed to DB
			fmt.Printf("redis error: %v\n", err)
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
		return "", fmt.Errorf("failed to get url from db: %w", err)
	}

	// 3. Update Redis - skip if redis is nil
	if r.redis != nil {
		// Set with expiration (e.g., 24 hours) to manage memory
		err = r.redis.Set(ctx, cacheKey, originalURL, 24*time.Hour).Err()
		if err != nil {
			fmt.Printf("failed to set cache: %v\n", err)
		}
	}

	return originalURL, nil
}

func (r *PostgresRedisRepository) Close() error {
	return r.db.Close()
}
