package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/hszk-dev/url-shortener/internal/shortener"
)

func TestShortenHandler(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		contentType    string
		mockSaveID     uint64
		mockSaveError  error
		expectedStatus int
		expectedFields []string
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "successful URL shortening",
			requestBody:    `{"url":"https://www.google.com"}`,
			contentType:    "application/json",
			mockSaveID:     1,
			mockSaveError:  nil,
			expectedStatus: http.StatusOK,
			expectedFields: []string{"short_code", "short_url"},
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp ShortenResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.ShortCode != "1" {
					t.Errorf("Expected short_code '1', got '%s'", resp.ShortCode)
				}
				if !strings.Contains(resp.ShortURL, "/1") {
					t.Errorf("Expected short_url to contain '/1', got '%s'", resp.ShortURL)
				}
			},
		},
		{
			name:           "empty URL",
			requestBody:    `{"url":""}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "URL is required") {
					t.Errorf("Expected 'URL is required' error, got: %s", body)
				}
			},
		},
		{
			name:           "invalid JSON",
			requestBody:    `{invalid json}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Invalid request body") {
					t.Errorf("Expected 'Invalid request body' error, got: %s", body)
				}
			},
		},
		{
			name:           "invalid URL scheme (ftp)",
			requestBody:    `{"url":"ftp://example.com"}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Invalid URL format") {
					t.Errorf("Expected 'Invalid URL format' error, got: %s", body)
				}
			},
		},
		{
			name:           "invalid URL (no scheme)",
			requestBody:    `{"url":"www.google.com"}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Invalid URL format") {
					t.Errorf("Expected 'Invalid URL format' error, got: %s", body)
				}
			},
		},
		{
			name:           "service returns error",
			requestBody:    `{"url":"https://www.example.com"}`,
			contentType:    "application/json",
			mockSaveID:     0,
			mockSaveError:  context.DeadlineExceeded,
			expectedStatus: http.StatusRequestTimeout,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Request timeout") {
					t.Errorf("Expected 'Request timeout' error, got: %s", body)
				}
			},
		},
		{
			name:           "HTTPS URL",
			requestBody:    `{"url":"https://github.com"}`,
			contentType:    "application/json",
			mockSaveID:     12345,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp ShortenResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.ShortCode != "3d7" {
					t.Errorf("Expected short_code '3d7', got '%s'", resp.ShortCode)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock repository
			mockRepo := &shortener.MockRepository{
				SaveFunc: func(ctx context.Context, url string) (uint64, error) {
					return tt.mockSaveID, tt.mockSaveError
				},
			}

			// Create app with mock service
			service := shortener.NewService(mockRepo)
			app := &App{
				Service: service,
				BaseURL: "http://localhost:8080",
			}

			// Create HTTP request
			req := httptest.NewRequest("POST", "/api/shorten", bytes.NewBufferString(tt.requestBody))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			app.ShortenHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Run custom response checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
				return // checkResponse handles all validation
			}

			// Check expected fields in JSON response (for success cases)
			if tt.expectedStatus == http.StatusOK && len(tt.expectedFields) > 0 {
				contentType := w.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
				}

				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode JSON response: %v", err)
				}

				for _, field := range tt.expectedFields {
					if _, exists := response[field]; !exists {
						t.Errorf("Expected field '%s' in response, but not found", field)
					}
				}
			}
		})
	}
}

func TestRedirectHandler(t *testing.T) {
	tests := []struct {
		name           string
		shortCode      string
		mockID         uint64
		mockURL        string
		mockError      error
		expectedStatus int
		expectedHeader string
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "successful redirect",
			shortCode:      "1",
			mockID:         1,
			mockURL:        "https://www.google.com",
			mockError:      nil,
			expectedStatus: http.StatusFound,
			expectedHeader: "https://www.google.com",
		},
		{
			name:           "URL not found",
			shortCode:      "xyz",
			mockError:      shortener.ErrNotFound,
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "URL not found") {
					t.Errorf("Expected 'URL not found' error, got: %s", body)
				}
			},
		},
		{
			name:           "invalid short code",
			shortCode:      "invalid!@#",
			mockError:      shortener.ErrInvalidShortCode,
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Invalid short code") {
					t.Errorf("Expected 'Invalid short code' error, got: %s", body)
				}
			},
		},
		{
			name:           "empty short code",
			shortCode:      "",
			mockError:      shortener.ErrInvalidShortCode,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "timeout error",
			shortCode:      "1",
			mockError:      context.DeadlineExceeded,
			expectedStatus: http.StatusRequestTimeout,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				body := strings.TrimSpace(w.Body.String())
				if !strings.Contains(body, "Request timeout") {
					t.Errorf("Expected 'Request timeout' error, got: %s", body)
				}
			},
		},
		{
			name:           "redirect with long URL",
			shortCode:      "3d7",
			mockID:         12345,
			mockURL:        "https://github.com/golang/go/issues/12345",
			mockError:      nil,
			expectedStatus: http.StatusFound,
			expectedHeader: "https://github.com/golang/go/issues/12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock repository
			mockRepo := &shortener.MockRepository{
				GetFunc: func(ctx context.Context, id uint64) (string, error) {
					return tt.mockURL, tt.mockError
				},
			}

			// Create app with mock service
			service := shortener.NewService(mockRepo)
			app := &App{
				Service: service,
				BaseURL: "http://localhost:8080",
			}

			// Create HTTP request with gorilla/mux variables
			req := httptest.NewRequest("GET", "/"+tt.shortCode, nil)
			w := httptest.NewRecorder()

			// Set up mux vars (simulate gorilla/mux path parameter)
			req = mux.SetURLVars(req, map[string]string{
				"shortCode": tt.shortCode,
			})

			// Call handler
			app.RedirectHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check Location header for successful redirects
			if tt.expectedStatus == http.StatusFound && tt.expectedHeader != "" {
				location := w.Header().Get("Location")
				if location != tt.expectedHeader {
					t.Errorf("Expected Location header '%s', got '%s'", tt.expectedHeader, location)
				}
			}

			// Run custom response checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestRedirectHandler_HTTP302(t *testing.T) {
	// Specific test to verify we use 302 Found (not 301 Moved Permanently)
	mockRepo := &shortener.MockRepository{
		GetFunc: func(ctx context.Context, id uint64) (string, error) {
			return "https://www.google.com", nil
		},
	}

	service := shortener.NewService(mockRepo)
	app := &App{
		Service: service,
		BaseURL: "http://localhost:8080",
	}

	req := httptest.NewRequest("GET", "/1", nil)
	req = mux.SetURLVars(req, map[string]string{"shortCode": "1"})
	w := httptest.NewRecorder()

	app.RedirectHandler(w, req)

	// Verify it's 302 Found (for analytics), not 301 Moved Permanently
	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302 Found, got %d", w.Code)
	}

	if w.Code == http.StatusMovedPermanently {
		t.Error("Should not use 301 Moved Permanently - this prevents analytics tracking")
	}
}

func TestShortenHandler_ContentType(t *testing.T) {
	// Test that response has correct Content-Type header
	mockRepo := &shortener.MockRepository{
		SaveFunc: func(ctx context.Context, url string) (uint64, error) {
			return 1, nil
		},
	}

	service := shortener.NewService(mockRepo)
	app := &App{
		Service: service,
		BaseURL: "http://localhost:8080",
	}

	req := httptest.NewRequest("POST", "/api/shorten",
		bytes.NewBufferString(`{"url":"https://www.google.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.ShortenHandler(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}
