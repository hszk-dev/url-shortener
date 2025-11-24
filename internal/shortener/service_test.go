package shortener

import (
	"context"
	"errors"
	"testing"
)

// MockRepository is a mock implementation of Repository for testing
type MockRepository struct {
	SaveFunc  func(ctx context.Context, originalURL string) (uint64, error)
	GetFunc   func(ctx context.Context, id uint64) (string, error)
	CloseFunc func() error
}

func (m *MockRepository) Save(ctx context.Context, originalURL string) (uint64, error) {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, originalURL)
	}
	return 0, nil
}

func (m *MockRepository) Get(ctx context.Context, id uint64) (string, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return "", nil
}

func (m *MockRepository) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func TestService_Shorten(t *testing.T) {
	tests := []struct {
		name        string
		originalURL string
		savedID     uint64
		saveError   error
		wantCode    string
		wantErr     bool
	}{
		{
			name:        "successful shortening",
			originalURL: "https://www.google.com",
			savedID:     1,
			saveError:   nil,
			wantCode:    "1",
			wantErr:     false,
		},
		{
			name:        "repository save error",
			originalURL: "https://example.com",
			savedID:     0,
			saveError:   errors.New("database error"),
			wantCode:    "",
			wantErr:     true,
		},
		{
			name:        "large ID encoding",
			originalURL: "https://github.com",
			savedID:     12345,
			saveError:   nil,
			wantCode:    "3d7",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				SaveFunc: func(ctx context.Context, url string) (uint64, error) {
					if url != tt.originalURL {
						t.Errorf("Save() called with wrong URL: got %s, want %s", url, tt.originalURL)
					}
					return tt.savedID, tt.saveError
				},
			}

			service := NewService(mockRepo)
			ctx := context.Background()

			gotCode, err := service.Shorten(ctx, tt.originalURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("Shorten() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && gotCode != tt.wantCode {
				t.Errorf("Shorten() = %s, want %s", gotCode, tt.wantCode)
			}
		})
	}
}

func TestService_Redirect(t *testing.T) {
	tests := []struct {
		name        string
		shortCode   string
		storedURL   string
		getError    error
		wantURL     string
		wantErr     bool
	}{
		{
			name:      "successful redirect",
			shortCode: "b",
			storedURL: "https://www.google.com",
			getError:  nil,
			wantURL:   "https://www.google.com",
			wantErr:   false,
		},
		{
			name:      "URL not found",
			shortCode: "xyz",
			storedURL: "",
			getError:  ErrNotFound,
			wantURL:   "",
			wantErr:   true,
		},
		{
			name:      "invalid short code",
			shortCode: "invalid!",
			storedURL: "",
			getError:  nil,
			wantURL:   "",
			wantErr:   true,
		},
		{
			name:      "repository error",
			shortCode: "c",
			storedURL: "",
			getError:  errors.New("database connection error"),
			wantURL:   "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				GetFunc: func(ctx context.Context, id uint64) (string, error) {
					return tt.storedURL, tt.getError
				},
			}

			service := NewService(mockRepo)
			ctx := context.Background()

			gotURL, err := service.Redirect(ctx, tt.shortCode)

			if (err != nil) != tt.wantErr {
				t.Errorf("Redirect() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && gotURL != tt.wantURL {
				t.Errorf("Redirect() = %s, want %s", gotURL, tt.wantURL)
			}
		})
	}
}

func TestService_RoundTrip(t *testing.T) {
	// Test the complete flow: Shorten -> Redirect
	originalURL := "https://www.example.com"
	var savedID uint64

	mockRepo := &MockRepository{
		SaveFunc: func(ctx context.Context, url string) (uint64, error) {
			savedID = 42
			return savedID, nil
		},
		GetFunc: func(ctx context.Context, id uint64) (string, error) {
			if id == savedID {
				return originalURL, nil
			}
			return "", ErrNotFound
		},
	}

	service := NewService(mockRepo)
	ctx := context.Background()

	// Step 1: Shorten
	shortCode, err := service.Shorten(ctx, originalURL)
	if err != nil {
		t.Fatalf("Shorten() failed: %v", err)
	}

	// Step 2: Redirect
	retrievedURL, err := service.Redirect(ctx, shortCode)
	if err != nil {
		t.Fatalf("Redirect() failed: %v", err)
	}

	if retrievedURL != originalURL {
		t.Errorf("Round trip failed: got %s, want %s", retrievedURL, originalURL)
	}
}
