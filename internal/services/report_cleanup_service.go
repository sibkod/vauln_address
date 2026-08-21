package services

import (
	"context"
	"log"
	"time"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

// ReportCleanupService deletes anonymous reports (check_history rows with an
// "ip:" requester) once they are older than models.AnonymousReportTTL.
// Authenticated users keep their reports because their history is never
// deleted.
type ReportCleanupService struct {
	repo     *repository.Repository
	stopChan chan struct{}
}

func NewReportCleanupService(repo *repository.Repository) *ReportCleanupService {
	return &ReportCleanupService{
		repo:     repo,
		stopChan: make(chan struct{}),
	}
}

func (s *ReportCleanupService) Start() {
	go s.run()
}

func (s *ReportCleanupService) Stop() {
	close(s.stopChan)
}

func (s *ReportCleanupService) run() {
	s.cleanup()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopChan:
			return
		}
	}
}

func (s *ReportCleanupService) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().Add(-models.AnonymousReportTTL)
	count, err := s.repo.DeleteExpiredAnonymousReports(ctx, cutoff)
	if err != nil {
		log.Printf("ReportCleanupService: failed to delete expired reports: %v", err)
		return
	}
	if count > 0 {
		log.Printf("ReportCleanupService: deleted %d expired anonymous reports", count)
	}
}
