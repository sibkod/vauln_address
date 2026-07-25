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
	WalletAddress string     `json:"wallet_address"`
	Chain         Chain      `json:"chain"`
	Nonce         string     `json:"-"`
	Balance       int        `json:"balance"` // Free checks remaining
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

// Web3 Auth Request/Response
type AuthRequest struct {
	Address string `json:"address" binding:"required"`
	Chain   string `json:"chain" binding:"required"`
	Signature string `json:"signature" binding:"required"`
	Message  string `json:"message" binding:"required"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	User     *UserPublic `json:"user"`
	ExpiresIn int    `json:"expires_in"`
}

type UserPublic struct {
	WalletAddress string `json:"wallet_address"`
	Chain         string `json:"chain"`
	Balance       int    `json:"balance"`
	IsPremium     bool   `json:"is_premium"`
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
	ChecksCount   int    `json:"checks" binding:"required,min=1"`
	Chain         string `json:"chain" binding:"required"`
	WalletAddress string `json:"wallet_address" binding:"required"`
}

type PurchaseResponse struct {
	OrderID        string    `json:"order_id"`
	ChecksCount    int       `json:"checks_count"`
	TotalUSD       float64   `json:"total_usd"`
	Amount         string    `json:"amount"`        // SOL amount for payment
	PaymentAddress string    `json:"payment_address"`
	DueDate        time.Time `json:"due_date,omitempty"`
	Status         string    `json:"status"`
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
	OrderUUID       string     `json:"order_uuid"`
	WalletAddress   string     `json:"wallet_address"`
	Chain           string     `json:"chain"`
	ChecksCount     int        `json:"checks_count"`
	TotalUSD        float64    `json:"total_usd"`
	Currency        string     `json:"currency"`
	TokenAmount     float64    `json:"token_amount"`
	PaymentAddress  string     `json:"payment_address"`
	Status          string     `json:"status"`
	TxHash          string     `json:"tx_hash,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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

// ==================== API Keys ====================

type APIKey struct {
	ID            int64     `json:"id"`
	WalletAddress string    `json:"wallet_address"`
	KeyHash       string    `json:"-"`           // SHA-256 hash of the key (never exposed)
	KeyPrefix     string    `json:"key_prefix"`  // First 8 chars for identification
	Name          string    `json:"name"`        // User-defined name
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	IsRevoked     bool      `json:"is_revoked"`
}

type CreateAPIKeyRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ExpiresIn int    `json:"expires_in"` // Days until expiration, 0 = never expires
}

type APIKeyResponse struct {
	Key         string    `json:"key"`          // Full key shown only once!
	KeyPrefix   string    `json:"key_prefix"`
	Name        string    `json:"name"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type APIKeyListResponse struct {
	Keys []APIKey `json:"keys"`
}

// ==================== Renew API Key via Web3 ====================

type RenewAPIKeyRequest struct {
	Address   string `json:"address" binding:"required"`
	Chain     string `json:"chain" binding:"required"`
	Signature string `json:"signature" binding:"required"`
	Message   string `json:"message" binding:"required"`
	KeyID     int64  `json:"key_id" binding:"required"` // ID of the key to renew
}

// ==================== Pricing Plans ====================

const (
	// Price per check in USD
	PricePerCheckUSD = 0.10 // $0.10 per check
	
	// Token discount
	SUIDiscountPercent = 50 // 50% discount for SUI token payments
	
	// SUI token price estimate (mock - should be fetched from oracle)
	SUIUSDPrice = 1.50 // ~$1.50 per SUI

	// API Key settings
	APIKeyLength     = 32       // 32 bytes = 64 hex characters
	APIKeyPrefix     = "vkn_"   // vauln-key prefix for identification
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

// ==================== Leaked Keys ====================

type KeyType string

const (
	KeyTypeSeed        KeyType = "seed"
	KeyTypePrivateKey  KeyType = "private_key"
)

type LeakedKey struct {
	ID           int64     `json:"id"`
	WalletID    int64     `json:"wallet_id"`      // 0 = no seed/key found
	Address     string    `json:"address"`
	Chain       string    `json:"chain"`
	KeyType     KeyType   `json:"key_type"`      // 'seed' or 'private_key'
	KeyValue    string    `json:"-"`             // Never exposed
	Source      string    `json:"source,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// LeakedKeyPublic is the public-facing version (without sensitive data)
type LeakedKeyPublic struct {
	ID           int64     `json:"id"`
	WalletID    int64     `json:"wallet_id"`      // 0 = flagged by other triggers
	Address     string    `json:"address"`
	Chain       string    `json:"chain"`
	KeyType     KeyType   `json:"key_type"`
	Source      string    `json:"source,omitempty"`
	HasKey      bool      `json:"has_key"`        // True if seed/private key was found
	DiscoveredAt time.Time `json:"discovered_at,omitempty"`
}

// ToPublic converts LeakedKey to public version (hides sensitive data)
func (lk *LeakedKey) ToPublic() LeakedKeyPublic {
	return LeakedKeyPublic{
		ID:           lk.ID,
		WalletID:    lk.WalletID,
		Address:     lk.Address,
		Chain:       lk.Chain,
		KeyType:     lk.KeyType,
		Source:      lk.Source,
		HasKey:      lk.WalletID > 0, // Has key only if wallet_id > 0
		DiscoveredAt: lk.DiscoveredAt,
	}
}
