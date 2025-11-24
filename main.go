package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/suzukikyou/url-shortener/internal/shortener"
)

type App struct {
	Service *shortener.Service
	BaseURL string
}

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	ShortCode string `json:"short_code"`
	ShortURL  string `json:"short_url"`
}

func (a *App) ShortenHandler(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate URL
	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.ParseRequestURI(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "Invalid URL format. Must be http:// or https://", http.StatusBadRequest)
		return
	}

	// Set timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	shortCode, err := a.Service.Shorten(ctx, req.URL)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "Request timeout", http.StatusRequestTimeout)
			log.Printf("Shorten timeout: %v", err)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Printf("Shorten error: %v", err)
		return
	}

	resp := ShortenResponse{
		ShortCode: shortCode,
		ShortURL:  fmt.Sprintf("%s/%s", a.BaseURL, shortCode),
	}

	// Encode to buffer first to catch encoding errors before writing headers
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (a *App) RedirectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	// Set timeout for cache/database operations (shorter for redirects)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	originalURL, err := a.Service.Redirect(ctx, shortCode)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "Request timeout", http.StatusRequestTimeout)
			log.Printf("Redirect timeout for code %s: %v", shortCode, err)
			return
		}
		if errors.Is(err, shortener.ErrInvalidShortCode) {
			http.Error(w, "Invalid short code", http.StatusBadRequest)
			return
		}
		if errors.Is(err, shortener.ErrNotFound) {
			http.Error(w, "URL not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Printf("Redirect error: %v", err)
		return
	}

	// 302 Found for analytics
	http.Redirect(w, r, originalURL, http.StatusFound)
}

func main() {
	// Load .env (optional in CI/production environments)
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found (using environment variables): %v", err)
	}

	// Connect to PostgreSQL
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Connect to Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer redisClient.Close()

	// Get base URL for short URLs
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Initialize Service
	repo := shortener.NewPostgresRedisRepository(db, redisClient)
	service := shortener.NewService(repo)
	app := &App{
		Service: service,
		BaseURL: baseURL,
	}

	// Setup Router
	r := mux.NewRouter()
	r.HandleFunc("/api/shorten", app.ShortenHandler).Methods("POST")
	r.HandleFunc("/{shortCode}", app.RedirectHandler).Methods("GET")

	// Configure HTTP Server with timeouts
	port := "8080"
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
		// ReadTimeout covers the time from connection accepted to request body fully read
		ReadTimeout: 10 * time.Second,
		// WriteTimeout covers the time from end of request header read to end of response write
		WriteTimeout: 10 * time.Second,
		// IdleTimeout is the max time to wait for the next request when keep-alives are enabled
		IdleTimeout: 120 * time.Second,
	}

	// Start Server
	log.Printf("Server starting on port %s", port)
	log.Fatal(srv.ListenAndServe())
}
