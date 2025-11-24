//go:build e2e

package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestE2E_ShortenAndRedirect validates the complete happy path:
// 1. POST /api/shorten creates a short URL
// 2. GET /{shortCode} redirects to the original URL with 302 Found
//
// This test verifies the core architectural decision:
// "Use 302 Found for redirects to enable server-side analytics"
func TestE2E_ShortenAndRedirect(t *testing.T) {
	testURL := "https://github.com/testcontainers/testcontainers-go"

	// Step 1: Shorten URL
	payload := fmt.Sprintf(`{"url":"%s"}`, testURL)
	resp := postJSON(t, BaseURL+"/api/shorten", payload)
	defer resp.Body.Close()

	// Verify response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200 OK, got %d: %s", resp.StatusCode, body)
	}

	// Verify Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Parse response
	var result struct {
		ShortCode string `json:"short_code"`
		ShortURL  string `json:"short_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response fields
	if result.ShortCode == "" {
		t.Fatal("short_code is empty")
	}
	if result.ShortURL == "" {
		t.Fatal("short_url is empty")
	}
	if !strings.HasSuffix(result.ShortURL, "/"+result.ShortCode) {
		t.Errorf("short_url should end with /%s, got %s", result.ShortCode, result.ShortURL)
	}

	t.Logf("Created short URL: %s -> %s", result.ShortCode, testURL)

	// Step 2: Access short URL and verify redirect
	redirectResp := getWithoutRedirect(t, BaseURL+"/"+result.ShortCode)
	defer redirectResp.Body.Close()

	// Verify 302 Found (not 301 Moved Permanently)
	if redirectResp.StatusCode != http.StatusFound {
		t.Errorf("Expected 302 Found, got %d", redirectResp.StatusCode)
	}

	// This check is critical for the core architectural decision
	if redirectResp.StatusCode == http.StatusMovedPermanently {
		t.Error("Should not use 301 Moved Permanently - this prevents analytics tracking")
	}

	// Verify Location header
	location := redirectResp.Header.Get("Location")
	if location != testURL {
		t.Errorf("Location header = %s, want %s", location, testURL)
	}

	t.Logf("Redirect verified: %s -> %s (HTTP %d)", result.ShortCode, location, redirectResp.StatusCode)
}

// TestE2E_ErrorHandling validates error cases and proper HTTP status codes.
//
// This test ensures the API follows HTTP semantics and provides clear error messages
// for various invalid input scenarios.
func TestE2E_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		expectedStatus int
		errorContains  string
	}{
		{
			name:           "Invalid URL format",
			payload:        `{"url":"not-a-valid-url"}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "Invalid URL format",
		},
		{
			name:           "Missing URL field",
			payload:        `{}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "URL is required",
		},
		{
			name:           "Empty URL",
			payload:        `{"url":""}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "URL is required",
		},
		{
			name:           "Malformed JSON",
			payload:        `{"url": invalid-json}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "Invalid request body",
		},
		{
			name:           "FTP scheme not allowed",
			payload:        `{"url":"ftp://example.com"}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "Invalid URL format",
		},
		{
			name:           "URL without scheme",
			payload:        `{"url":"www.google.com"}`,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "Invalid URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSON(t, BaseURL+"/api/shorten", tt.payload)
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}

			body, _ := io.ReadAll(resp.Body)
			bodyStr := strings.TrimSpace(string(body))

			if !strings.Contains(bodyStr, tt.errorContains) {
				t.Errorf("Error message does not contain '%s': %s", tt.errorContains, bodyStr)
			}

			t.Logf("Error case handled correctly: %s -> %d: %s", tt.name, resp.StatusCode, bodyStr)
		})
	}
}

// TestE2E_NonExistentShortCode validates 404 behavior for non-existent short codes.
func TestE2E_NonExistentShortCode(t *testing.T) {
	// Use a short code that is extremely unlikely to exist
	nonExistentCode := "zzzZZZ999"

	resp := getWithoutRedirect(t, BaseURL+"/"+nonExistentCode)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "URL not found") {
		t.Errorf("Expected 'URL not found' error message, got: %s", body)
	}

	t.Logf("404 behavior verified for non-existent code: %s", nonExistentCode)
}

// TestE2E_CacheBehavior validates the Read-Through caching pattern.
//
// This test verifies:
//   - First access (cache miss) succeeds
//   - Second access (cache hit) succeeds and is typically faster
//   - Both requests return identical results
//
// Note: We don't enforce strict performance requirements in E2E tests
// due to network/container variance, but log the timing for analysis.
func TestE2E_CacheBehavior(t *testing.T) {
	testURL := "https://example.com/cache-test-e2e"

	// Create short URL
	payload := fmt.Sprintf(`{"url":"%s"}`, testURL)
	resp := postJSON(t, BaseURL+"/api/shorten", payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to create short URL: %d", resp.StatusCode)
	}

	var result struct {
		ShortCode string `json:"short_code"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	// First access (cache miss → DB → Redis)
	start := time.Now()
	resp1 := getWithoutRedirect(t, BaseURL+"/"+result.ShortCode)
	firstAccess := time.Since(start)
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusFound {
		t.Fatalf("First access failed: %d", resp1.StatusCode)
	}

	location1 := resp1.Header.Get("Location")

	// Small delay to ensure cache is populated
	time.Sleep(100 * time.Millisecond)

	// Second access (cache hit → Redis only)
	start = time.Now()
	resp2 := getWithoutRedirect(t, BaseURL+"/"+result.ShortCode)
	secondAccess := time.Since(start)
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusFound {
		t.Fatalf("Second access failed: %d", resp2.StatusCode)
	}

	location2 := resp2.Header.Get("Location")

	// Verify both redirects point to the same URL
	if location1 != location2 {
		t.Errorf("Inconsistent redirects: first=%s, second=%s", location1, location2)
	}

	if location1 != testURL {
		t.Errorf("Redirect location = %s, want %s", location1, testURL)
	}

	// Log performance metrics (informational only)
	t.Logf("Cache behavior verified:")
	t.Logf("  First access (cache miss):  %v", firstAccess)
	t.Logf("  Second access (cache hit):  %v", secondAccess)

	if secondAccess < firstAccess {
		t.Logf("  ✓ Cache hit was faster (expected)")
	} else {
		t.Logf("  ⚠ Cache hit was not faster (may be network variance)")
	}
}

// TestE2E_ConcurrentRequests validates system behavior under concurrent load.
//
// This test verifies:
//   - Multiple concurrent requests all succeed
//   - Each request gets a unique short code (BIGSERIAL guarantees)
//   - No race conditions or data corruption
//
// This validates the architectural decision:
// "Base62 encoding with Database Auto-Increment guarantees uniqueness"
func TestE2E_ConcurrentRequests(t *testing.T) {
	const numWorkers = 50

	results := make(chan string, numWorkers)
	errors := make(chan error, numWorkers)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	startTime := time.Now()

	// Launch concurrent requests
	for i := 0; i < numWorkers; i++ {
		go func(n int) {
			defer wg.Done()

			testURL := fmt.Sprintf("https://example.com/concurrent-e2e/%d", n)
			payload := fmt.Sprintf(`{"url":"%s"}`, testURL)

			resp, err := http.Post(
				BaseURL+"/api/shorten",
				"application/json",
				strings.NewReader(payload),
			)
			if err != nil {
				errors <- fmt.Errorf("worker %d: request failed: %w", n, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				errors <- fmt.Errorf("worker %d: got status %d: %s", n, resp.StatusCode, body)
				return
			}

			var result struct {
				ShortCode string `json:"short_code"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				errors <- fmt.Errorf("worker %d: decode failed: %w", n, err)
				return
			}

			results <- result.ShortCode
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	duration := time.Since(startTime)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent request failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Failed with %d errors out of %d requests", errorCount, numWorkers)
	}

	// Verify all short codes are unique
	seen := make(map[string]bool)
	for code := range results {
		if seen[code] {
			t.Errorf("Duplicate short code detected: %s", code)
		}
		seen[code] = true
	}

	if len(seen) != numWorkers {
		t.Errorf("Expected %d unique short codes, got %d", numWorkers, len(seen))
	}

	// Performance metrics
	avgLatency := duration / time.Duration(numWorkers)
	t.Logf("Concurrent requests validated:")
	t.Logf("  Total requests: %d", numWorkers)
	t.Logf("  Unique codes:   %d", len(seen))
	t.Logf("  Total time:     %v", duration)
	t.Logf("  Avg latency:    %v", avgLatency)
	t.Logf("  Throughput:     %.2f req/sec", float64(numWorkers)/duration.Seconds())
}

// TestE2E_MultipleURLsSameTarget validates that shortening the same URL
// multiple times produces different short codes.
//
// This is the expected behavior with the current implementation:
// - Each POST creates a new DB entry with a new ID
// - The same original URL can have multiple short codes
//
// Alternative design (URL deduplication) would require:
// - UNIQUE constraint on original_url column
// - SELECT before INSERT to check for existing URLs
// - Trade-off: Saves storage but adds DB query overhead
func TestE2E_MultipleURLsSameTarget(t *testing.T) {
	testURL := "https://example.com/duplicate-test"

	shortCodes := make([]string, 3)

	// Create 3 short URLs for the same original URL
	for i := 0; i < 3; i++ {
		payload := fmt.Sprintf(`{"url":"%s"}`, testURL)
		resp := postJSON(t, BaseURL+"/api/shorten", payload)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Request %d failed: %d", i+1, resp.StatusCode)
		}

		var result struct {
			ShortCode string `json:"short_code"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		shortCodes[i] = result.ShortCode
	}

	// Verify all short codes are different
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if shortCodes[i] == shortCodes[j] {
				t.Errorf("Short codes should be unique: codes[%d]=%s equals codes[%d]=%s",
					i, shortCodes[i], j, shortCodes[j])
			}
		}
	}

	// Verify all short codes redirect to the same URL
	for i, code := range shortCodes {
		resp := getWithoutRedirect(t, BaseURL+"/"+code)
		location := resp.Header.Get("Location")
		resp.Body.Close()

		if location != testURL {
			t.Errorf("Short code %d (%s) redirects to %s, want %s", i+1, code, location, testURL)
		}
	}

	t.Logf("Same URL creates unique short codes: %v", shortCodes)
}