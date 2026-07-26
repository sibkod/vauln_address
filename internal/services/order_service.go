package services

import (
	"context"
	"log"
	"time"

	"vauln-address/internal/repository"
)

// OrderExpirationDuration is how long an order stays pending before expiring
const OrderExpirationDuration = 30 * time.Minute

// OrderService handles order lifecycle events
type OrderService struct {
	repo      *repository.Repository
	stopChan  chan struct{}
	expiration time.Duration
}

// NewOrderService creates a new order service
func NewOrderService(repo *repository.Repository) *OrderService {
	return &OrderService{
		repo:      repo,
		stopChan:  make(chan struct{}),
		expiration: OrderExpirationDuration,
	}
}

// Start begins the background expiration checker
func (s *OrderService) Start() {
	go s.runExpirationChecker()
}

// Stop stops the order service
func (s *OrderService) Stop() {
	close(s.stopChan)
}

func (s *OrderService) runExpirationChecker() {
	// Run immediately on start
	s.expireOldOrders()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.expireOldOrders()
		case <-s.stopChan:
			return
		}
	}
}

func (s *OrderService) expireOldOrders() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := s.repo.ExpireOrders(ctx, s.expiration)
	if err != nil {
		log.Printf("OrderService: failed to expire orders: %v", err)
		return
	}

	if count > 0 {
		log.Printf("OrderService: expired %d orders older than %v", count, s.expiration)
	}
}
