package shortener

import "context"

// MockRepository is a mock implementation of Repository for testing.
// This mock is exported to allow usage in tests across multiple packages.
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
