package services

import (
	"context"
	"log"
	"time"

	"vauln-address/internal/repository"
)

// RateLimitResetService handles daily rate limit resets at 00:00 UTC
type RateLimitResetService struct {
	repo     *repository.Repository
	stopChan chan struct{}
}

// NewRateLimitResetService creates a new rate limit reset service
func NewRateLimitResetService(repo *repository.Repository) *RateLimitResetService {
	return &RateLimitResetService{
		repo:     repo,
		stopChan: make(chan struct{}),
	}
}

// Start begins the background rate limit reset scheduler
func (s *RateLimitResetService) Start() {
	go s.runResetScheduler()
}

// Stop stops the rate limit reset service
func (s *RateLimitResetService) Stop() {
	close(s.stopChan)
}

func (s *RateLimitResetService) runResetScheduler() {
	// Calculate time until next 00:00 UTC
	nextReset := s.nextMidnightUTC()
	waitDuration := time.Until(nextReset)
	
	log.Printf("RateLimitResetService: next reset scheduled at %s (in %s)", nextReset.Format(time.RFC3339), waitDuration)
	
	// Wait until next midnight
	select {
	case <-time.After(waitDuration):
		s.resetAllRateLimits()
	case <-s.stopChan:
		return
	}

	// Run reset check every hour to catch the midnight window
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now().UTC()
			// Check if we're within the first minute of a new day
			if now.Hour() == 0 && now.Minute() == 0 {
				s.resetAllRateLimits()
			}
		case <-s.stopChan:
			return
		}
	}
}

// nextMidnightUTC returns the next occurrence of midnight UTC
func (s *RateLimitResetService) nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return tomorrow
}

func (s *RateLimitResetService) resetAllRateLimits() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := s.repo.ResetAllRateLimits(ctx)
	if err != nil {
		log.Printf("RateLimitResetService: failed to reset rate limits: %v", err)
		return
	}

	log.Printf("RateLimitResetService: reset rate limits for %d IPs", count)
}
