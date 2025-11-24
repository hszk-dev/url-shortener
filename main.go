package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

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

	shortCode, err := a.Service.Shorten(r.Context(), req.URL)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Printf("Shorten error: %v", err)
		return
	}

	resp := ShortenResponse{
		ShortCode: shortCode,
		ShortURL:  fmt.Sprintf("%s/%s", a.BaseURL, shortCode),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *App) RedirectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	originalURL, err := a.Service.Redirect(r.Context(), shortCode)
	if err != nil {
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
	// Load .env
	_ = godotenv.Load()

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

	// Start Server
	port := "8080"
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
