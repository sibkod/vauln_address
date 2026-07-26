package services

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"vauln-address/internal/config"
)

// PriceService fetches and caches SOL price from CoinGecko
type PriceService struct {
	cfg       *config.Config
	solPrice  float64
	updatedAt time.Time
	mu        sync.RWMutex
	stopChan  chan struct{}
}

// PriceResponse from CoinGecko
type PriceResponse struct {
	Solana struct {
		USD float64 `json:"usd"`
	} `json:"solana"`
}

func NewPriceService(cfg *config.Config) *PriceService {
	ps := &PriceService{
		cfg:      cfg,
		solPrice: cfg.SolanaPriceUSD,
		stopChan: make(chan struct{}),
	}
	go ps.startUpdater()
	return ps
}

func (ps *PriceService) startUpdater() {
	// Initial fetch
	ps.fetchAndUpdate()

	// Update every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ps.fetchAndUpdate()
		case <-ps.stopChan:
			return
		}
	}
}

func (ps *PriceService) fetchAndUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := "https://api.coingecko.com/api/v3/simple/price?ids=solana&vs_currencies=usd"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("PriceService: failed to create request: %v", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("PriceService: failed to fetch price: %v", err)
		return
	}
	defer resp.Body.Close()

	var priceResp PriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&priceResp); err != nil {
		log.Printf("PriceService: failed to decode response: %v", err)
		return
	}

	if priceResp.Solana.USD > 0 {
		ps.mu.Lock()
		ps.solPrice = priceResp.Solana.USD
		ps.updatedAt = time.Now()
		ps.mu.Unlock()
		log.Printf("PriceService: SOL price updated to $%.2f", priceResp.Solana.USD)
	}
}

// GetSolPrice returns current SOL price in USD
func (ps *PriceService) GetSolPrice() float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.solPrice
}

// GetUpdatedAt returns when the price was last updated
func (ps *PriceService) GetUpdatedAt() time.Time {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.updatedAt
}

// Stop stops the price updater
func (ps *PriceService) Stop() {
	close(ps.stopChan)
}
