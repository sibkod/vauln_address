package models

import "time"

type WalletStatus string

const (
	StatusHacked     WalletStatus = "hacked"
	StatusVulnerable WalletStatus = "vulnerable"
	StatusSafe       WalletStatus = "safe"
	StatusHacker     WalletStatus = "hacker"
	StatusDrained    WalletStatus = "drained"
)

type Chain string

const (
	ChainEVM    Chain = "evm"
	ChainBTC    Chain = "btc"
	ChainSolana Chain = "solana"
	ChainSui    Chain = "sui"
	ChainTron   Chain = "tron"
)

func IsValidChain(chain string) bool {
	switch Chain(chain) {
	case ChainEVM, ChainBTC, ChainSolana, ChainSui, ChainTron:
		return true
	}
	return false
}

func IsValidStatus(status string) bool {
	switch WalletStatus(status) {
	case StatusHacked, StatusVulnerable, StatusSafe, StatusHacker, StatusDrained:
		return true
	}
	return false
}

type Wallet struct {
	ID        int64       `json:"id"`
	Address   string      `json:"address"`
	Chain     Chain       `json:"chain"`
	Status    WalletStatus `json:"status"`
	HasPK     bool        `json:"has_pk"`
	HasSeed   bool        `json:"has_seed"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type ContactMessage struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type RateLimit struct {
	ID        int64     `json:"id"`
	IPAddress string    `json:"ip_address"`
	Count     int       `json:"count"`
	WindowStart time.Time `json:"window_start"`
}

type CheckRequest struct {
	Address string `json:"address" binding:"required"`
	Chain   string `json:"chain" binding:"required"`
}

type CheckResponse struct {
	Address   string      `json:"address"`
	Chain     string      `json:"chain"`
	Status    string      `json:"status"`
	HasPK     bool        `json:"has_pk"`
	HasSeed   bool        `json:"has_seed"`
	Found     bool        `json:"found"`
}

type ContactRequest struct {
	Name    string `json:"name" binding:"required"`
	Email   string `json:"email" binding:"required,email"`
	Message string `json:"message" binding:"required,min=1,max=5000"`
}

type RecentCheck struct {
	ID        int64     `json:"id"`
	Address   string    `json:"address"`
	Chain     string    `json:"chain"`
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}
