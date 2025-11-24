package shortener

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPostgresRedisRepository_Save(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name        string
		originalURL string
		wantID      uint64
		setupMock   func(sqlmock.Sqlmock)
		wantErr     bool
	}{
		{
			name:        "successful save",
			originalURL: "https://www.google.com",
			wantID:      1,
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
				m.ExpectQuery("INSERT INTO urls").
					WithArgs("https://www.google.com").
					WillReturnRows(rows)
			},
			wantErr: false,
		},
		{
			name:        "database error",
			originalURL: "https://example.com",
			wantID:      0,
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("INSERT INTO urls").
					WithArgs("https://example.com").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock(mock)

			// Use a nil Redis client for this test (we're only testing DB logic)
			repo := &PostgresRedisRepository{
				db:    db,
				redis: nil,
			}

			ctx := context.Background()
			gotID, err := repo.Save(ctx, tt.originalURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("Save() = %d, want %d", gotID, tt.wantID)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPostgresRedisRepository_Get_CacheHit(t *testing.T) {
	mr := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name        string
		id          uint64
		cachedURL   string
		expectQuery bool
	}{
		{
			name:        "cache hit - no DB query",
			id:          1,
			cachedURL:   "https://www.google.com",
			expectQuery: false,
		},
		{
			name:        "cache hit with different URL",
			id:          42,
			cachedURL:   "https://github.com",
			expectQuery: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-populate Redis cache
			cacheKey := fmt.Sprintf("shorturl:id:%d", tt.id)
			mr.Set(cacheKey, tt.cachedURL)

			// Expect NO database queries (cache hit)
			// sqlmock will fail if any unexpected query is executed

			repo := &PostgresRedisRepository{
				db:    db,
				redis: redisClient,
			}

			ctx := context.Background()
			gotURL, err := repo.Get(ctx, tt.id)

			if err != nil {
				t.Errorf("Get() unexpected error = %v", err)
				return
			}

			if gotURL != tt.cachedURL {
				t.Errorf("Get() = %s, want %s", gotURL, tt.cachedURL)
			}

			// Verify no DB queries were executed
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}

			// Clean up cache for next test
			mr.Del(cacheKey)
		})
	}
}

func TestPostgresRedisRepository_Get_CacheMiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name      string
		id        uint64
		setupMock func(sqlmock.Sqlmock)
		wantURL   string
		wantErr   error
	}{
		{
			name: "successful cache miss and DB retrieval",
			id:   1,
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"original_url"}).
					AddRow("https://www.google.com")
				m.ExpectQuery("SELECT original_url FROM urls WHERE id").
					WithArgs(int64(1)).
					WillReturnRows(rows)
			},
			wantURL: "https://www.google.com",
			wantErr: nil,
		},
		{
			name: "URL not found in database",
			id:   999,
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT original_url FROM urls WHERE id").
					WithArgs(int64(999)).
					WillReturnError(sql.ErrNoRows)
			},
			wantURL: "",
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock(mock)

			// Use nil Redis client to skip cache logic in tests
			// In production integration tests, use miniredis or testcontainers
			repo := &PostgresRedisRepository{
				db:    db,
				redis: nil,
			}

			ctx := context.Background()

			gotURL, err := repo.Get(ctx, tt.id)

			if err != tt.wantErr {
				t.Errorf("Get() error = %v, want %v", err, tt.wantErr)
				return
			}

			if gotURL != tt.wantURL {
				t.Errorf("Get() = %s, want %s", gotURL, tt.wantURL)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestBase62_Bijection(t *testing.T) {
	// Property test: encoding and decoding should be bijective
	testIDs := []uint64{0, 1, 10, 100, 1000, 10000, 100000, 1000000}

	for _, id := range testIDs {
		encoded := Encode(id)
		decoded, err := Decode(encoded)

		if err != nil {
			t.Errorf("Decode(%s) failed: %v", encoded, err)
		}

		if decoded != id {
			t.Errorf("Bijection failed: %d -> %s -> %d", id, encoded, decoded)
		}
	}
}
