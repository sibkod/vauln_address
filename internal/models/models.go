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

// ==================== User Authentication ====================

type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email,omitempty"`
	PasswordHash string     `json:"-"`
	Balance      int        `json:"balance"` // Free checks remaining
	IsPremium    bool       `json:"is_premium"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// Email/Password Auth Request/Response
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token     string       `json:"token"`
	User      *UserPublic  `json:"user"`
	ExpiresIn int          `json:"expires_in"`
}

type UserPublic struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Balance   int    `json:"balance"`
	IsPremium bool   `json:"is_premium"`
}

type NonceResponse struct {
	Nonce   string `json:"nonce"`
	Message string `json:"message"`
}

// ==================== Pricing & Payments ====================

type PaymentCurrency string

const (
	CurrencySUI     PaymentCurrency = "sui"
	CurrencyUSDC    PaymentCurrency = "usdc"
	CurrencyUSDT    PaymentCurrency = "usdt"
	CurrencyETH     PaymentCurrency = "eth"
)

type Pricing struct {
	ID              int64     `json:"id"`
	ChecksIncluded  int       `json:"checks_included"`
	PriceUSD        float64   `json:"price_usd"`
	DiscountPercent int       `json:"discount_percent"`
	TokenSymbol     string    `json:"token_symbol"`
	CreatedAt       time.Time `json:"created_at"`
}

type PaymentMethod struct {
	Currency      PaymentCurrency `json:"currency"`
	PriceUSD      float64         `json:"price_usd"`
	TokenAmount   float64         `json:"token_amount,omitempty"` // For crypto payments
	HasDiscount   bool             `json:"has_discount"`
	DiscountLabel string           `json:"discount_label,omitempty"`
}

type PurchaseRequest struct {
	ChecksCount int    `json:"checks_count" binding:"required,min=1,max=100"`
	Currency    string `json:"currency" binding:"required"`
}

type PurchaseResponse struct {
	OrderID     string `json:"order_id"`
	ChecksCount int    `json:"checks_count"`
	TotalUSD    float64 `json:"total_usd"`
	Currency    string `json:"currency"`
	TokenAmount float64 `json:"token_amount,omitempty"`
	PaymentAddress string `json:"payment_address,omitempty"`
	DueDate     time.Time `json:"due_date,omitempty"`
	Status      string `json:"status"`
}

// Payment Status
type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentCompleted  PaymentStatus = "completed"
	PaymentFailed    PaymentStatus = "failed"
	PaymentCancelled PaymentStatus = "cancelled"
)

type Order struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	OrderUUID       string    `json:"order_uuid"`
	ChecksCount     int       `json:"checks_count"`
	TotalUSD        float64   `json:"total_usd"`
	Currency        string    `json:"currency"`
	TokenAmount     float64   `json:"token_amount"`
	PaymentAddress  string    `json:"payment_address"`
	Status          string    `json:"status"`
	TxHash          string    `json:"tx_hash,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// ==================== Existing Models ====================

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
	Address     string      `json:"address"`
	Chain       string      `json:"chain"`
	Status      string      `json:"status"`
	HasPK       bool        `json:"has_pk"`
	HasSeed     bool        `json:"has_seed"`
	Found       bool        `json:"found"`
	BalanceLeft int         `json:"balance_left,omitempty"`
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

// ==================== Pricing Plans ====================

const (
	// Price per check in USD
	PricePerCheckUSD = 0.10 // $0.10 per check
	
	// Token discount
	SUIDiscountPercent = 50 // 50% discount for SUI token payments
	
	// SUI token price estimate (mock - should be fetched from oracle)
	SUIUSDPrice = 1.50 // ~$1.50 per SUI
)

// GetPricing returns pricing for different currencies
func GetPricing(checksCount int) []PaymentMethod {
	basePrice := float64(checksCount) * PricePerCheckUSD
	
	return []PaymentMethod{
		{
			Currency:      CurrencyUSDC,
			PriceUSD:      basePrice,
			HasDiscount:   false,
			DiscountLabel: "",
		},
		{
			Currency:      CurrencyUSDT,
			PriceUSD:      basePrice,
			HasDiscount:   false,
			DiscountLabel: "",
		},
		{
			Currency:      CurrencyETH,
			PriceUSD:      basePrice,
			TokenAmount:   basePrice / 2000, // ~$2000/ETH
			HasDiscount:   false,
			DiscountLabel: "",
		},
		{
			Currency:      CurrencySUI,
			PriceUSD:      basePrice,
			TokenAmount:   (basePrice * (100 + SUIDiscountPercent)) / 100 / SUIUSDPrice,
			HasDiscount:   true,
			DiscountLabel: "50% OFF",
		},
	}
}
