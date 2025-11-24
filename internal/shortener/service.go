package shortener

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidShortCode = errors.New("invalid short code")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Shorten(ctx context.Context, originalURL string) (string, error) {
	// 1. Save to DB to get unique ID
	id, err := s.repo.Save(ctx, originalURL)
	if err != nil {
		return "", fmt.Errorf("failed to save url: %w", err)
	}

	// 2. Encode ID to Base62
	shortCode := Encode(id)

	return shortCode, nil
}

func (s *Service) Redirect(ctx context.Context, shortCode string) (string, error) {
	// 1. Decode Base62 to ID
	id, err := Decode(shortCode)
	if err != nil {
		return "", ErrInvalidShortCode
	}

	// 2. Get Original URL from Repo (Redis/DB)
	originalURL, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err // Pass through ErrNotFound or other errors
	}

	return originalURL, nil
}
