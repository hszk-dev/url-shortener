//go:build e2e

package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	// BaseURL is the URL where the service is expected to be running
	BaseURL = "http://localhost:8080"
	// ServiceReadyTimeout is the maximum time to wait for service to become ready
	ServiceReadyTimeout = 30 * time.Second
)

// waitForService waits for the service to be ready by polling the health endpoint.
// This is critical for E2E tests to ensure docker-compose services are fully started.
//
// Implementation uses exponential backoff to reduce log noise during container startup.
func waitForService(t *testing.T, url string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++

		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			t.Logf("Service ready after %d attempts", attempt)
			return
		}

		if resp != nil {
			resp.Body.Close()
		}

		// Exponential backoff: 100ms, 200ms, 400ms, 800ms, max 2s
		backoff := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}

		time.Sleep(backoff)
	}

	t.Fatalf("Service at %s did not become ready within %v (attempts: %d)", url, timeout, attempt)
}

// postJSON sends a POST request with JSON payload and returns the response.
// The caller is responsible for closing the response body.
//
// Usage:
//
//	resp := postJSON(t, "http://localhost:8080/api/shorten", `{"url":"https://example.com"}`)
//	defer resp.Body.Close()
func postJSON(t *testing.T, url, payload string) *http.Response {
	t.Helper()

	resp, err := http.Post(url, "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST request to %s failed: %v", url, err)
	}

	return resp
}

// getWithoutRedirect sends a GET request and prevents following redirects.
// This is essential for testing redirect behavior (302 status codes).
//
// Usage:
//
//	resp := getWithoutRedirect(t, "http://localhost:8080/abc123")
//	defer resp.Body.Close()
//	if resp.StatusCode != http.StatusFound {
//	    t.Errorf("Expected 302 Found, got %d", resp.StatusCode)
//	}
func getWithoutRedirect(t *testing.T, url string) *http.Response {
	t.Helper()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Stop after first response
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET request to %s failed: %v", url, err)
	}

	return resp
}

// TestMain sets up and tears down the E2E test environment.
//
// Prerequisites:
//   - docker-compose up -d must be run before tests
//   - Services must expose ports as defined in docker-compose.yml
//
// This function verifies the service is ready before running any tests.
func TestMain(m *testing.M) {
	// Check if service is already running
	resp, err := http.Get(BaseURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "=======================================================================\n")
		fmt.Fprintf(os.Stderr, "ERROR: Service not accessible at %s\n", BaseURL)
		fmt.Fprintf(os.Stderr, "=======================================================================\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "E2E tests require docker-compose services to be running.\n")
		fmt.Fprintf(os.Stderr, "Please run the following command before executing E2E tests:\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "    docker-compose up -d\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Then wait a few seconds for services to initialize, and run:\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "    go test -tags=e2e -v ./tests/\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "=======================================================================\n")
		os.Exit(1)
	}
	if resp != nil {
		resp.Body.Close()
	}

	fmt.Printf("Service is ready at %s\n", BaseURL)

	// Run tests
	exitCode := m.Run()

	// Cleanup (optional - leave services running for debugging)
	// If you want to automatically stop services after tests:
	// exec.Command("docker-compose", "down").Run()

	os.Exit(exitCode)
}
