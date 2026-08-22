package models

import "time"

type WalletStatus string

const (
	StatusHacked     WalletStatus = "hacked"
	StatusVulnerable WalletStatus = "vulnerable"
	StatusSafe       WalletStatus = "safe"
	StatusHacker     WalletStatus = "hacker"
	StatusDrained    WalletStatus = "drained"
	StatusPhishing   WalletStatus = "phishing"
	StatusScam       WalletStatus = "scam"
	StatusMixer      WalletStatus = "mixer"
	StatusSanctioned WalletStatus = "sanctioned"
	StatusExchange   WalletStatus = "exchange"
	StatusSuspicious WalletStatus = "suspicious"
	StatusFrozen     WalletStatus = "frozen"
	StatusUnknown    WalletStatus = "unknown"
)

// StatusSeverity groups wallet statuses by how dangerous an address is.
const (
	SeverityDanger  = "danger"  // malicious or compromised — never interact
	SeverityWarning = "warning" // suspicious or risky — interact with caution
	SeverityInfo    = "info"    // neutral informational listing
)

// StatusInfo describes one wallet status for API consumers and the UI.
type StatusInfo struct {
	Status      WalletStatus `json:"status"`
	Label       string       `json:"label"`
	Severity    string       `json:"severity"`
	Description string       `json:"description"`
}

// statusCatalog is the single source of truth for wallet statuses:
// validation, admin import errors, report details and the /api/statuses
// endpoint all derive from it.
var statusCatalog = []StatusInfo{
	{StatusHacked, "Hacked", SeverityDanger,
		"The wallet is compromised: its private key or seed phrase is publicly known. Anyone can import it and steal all funds — do not use this address."},
	{StatusVulnerable, "Vulnerable", SeverityWarning,
		"Wallet data was exposed, but the funds have not been stolen yet. Move all assets to a new wallet immediately."},
	{StatusSafe, "Safe", SeverityInfo,
		"The address is listed in the database as safe: no leaks or malicious activity detected."},
	{StatusHacker, "Hacker", SeverityDanger,
		"The address belongs to a known hacker or drainer operator and is linked to the theft of funds. Never send assets to it."},
	{StatusDrained, "Drained", SeverityDanger,
		"The wallet was compromised and all funds were withdrawn from it."},
	{StatusPhishing, "Phishing", SeverityDanger,
		"The address is linked to a phishing campaign: it collects funds from victims of fake websites and cloned services."},
	{StatusScam, "Scam", SeverityDanger,
		"The address is linked to a fraudulent scheme: fake investments, giveaway scams or impersonation."},
	{StatusMixer, "Mixer", SeverityWarning,
		"The address is used to mix or launder funds and obscure their origin; interacting with it may flag your wallet in compliance checks."},
	{StatusSanctioned, "Sanctioned", SeverityDanger,
		"The address is listed in sanctions registries (e.g. OFAC SDN). Interacting with it may have legal consequences."},
	{StatusExchange, "Exchange", SeverityInfo,
		"Verified deposit address of a known exchange or custodial service."},
	{StatusSuspicious, "Suspicious", SeverityWarning,
		"Drainer-like activity was detected for this address, but it is not confirmed as malicious yet. Treat it with caution."},
	{StatusFrozen, "Frozen", SeverityWarning,
		"Assets on this address were frozen by the token issuer or by a court order; outgoing transfers may be blocked."},
	{StatusUnknown, "Unknown", SeverityWarning,
		"No verdict on this wallet yet. It appeared in the database because funds were moved to a known hacker or drainer operator — treat it as potentially compromised."},
}

// StatusInfos returns the full catalog of known wallet statuses.
func StatusInfos() []StatusInfo {
	out := make([]StatusInfo, len(statusCatalog))
	copy(out, statusCatalog)
	return out
}

// StatusDescription returns the human-readable explanation of a status.
// The second return value is false for unknown statuses.
func StatusDescription(status WalletStatus) (string, bool) {
	for _, info := range statusCatalog {
		if info.Status == status {
			return info.Description, true
		}
	}
	return "", false
}

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
	_, ok := StatusDescription(WalletStatus(status))
	return ok
}

// ValidStatusNames returns the list of accepted status values, e.g. for
// error messages of the admin import endpoints.
func ValidStatusNames() []string {
	names := make([]string, len(statusCatalog))
	for i, info := range statusCatalog {
		names[i] = string(info.Status)
	}
	return names
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
	Address   string `json:"address" binding:"required"`
	Chain     string `json:"chain" binding:"required"`
	Signature string `json:"signature" binding:"required"`
	Message   string `json:"message" binding:"required"`
}

type AuthResponse struct {
	Token     string      `json:"token"`
	User      *UserPublic `json:"user"`
	ExpiresIn int         `json:"expires_in"`
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
	CurrencySUI  PaymentCurrency = "sui"
	CurrencyUSDC PaymentCurrency = "usdc"
	CurrencyUSDT PaymentCurrency = "usdt"
	CurrencyETH  PaymentCurrency = "eth"
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
	HasDiscount   bool            `json:"has_discount"`
	DiscountLabel string          `json:"discount_label,omitempty"`
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
	Amount         string    `json:"amount"` // SOL amount for payment
	PaymentAddress string    `json:"payment_address"`
	DueDate        time.Time `json:"due_date,omitempty"`
	Status         string    `json:"status"`
}

// Payment Status
type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentCompleted PaymentStatus = "completed"
	PaymentFailed    PaymentStatus = "failed"
	PaymentCancelled PaymentStatus = "cancelled"
	PaymentExpired   PaymentStatus = "expired"
)

type Order struct {
	OrderUUID      string     `json:"order_uuid"`
	WalletAddress  string     `json:"wallet_address"`
	Chain          string     `json:"chain"`
	ChecksCount    int        `json:"checks_count"`
	TotalUSD       float64    `json:"total_usd"`
	Currency       string     `json:"currency"`
	TokenAmount    float64    `json:"token_amount"`
	PaymentAddress string     `json:"payment_address"`
	Status         string     `json:"status"`
	TxHash         string     `json:"tx_hash,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// ==================== Existing Models ====================

type Wallet struct {
	ID      int64        `json:"id"`
	Address string       `json:"address"`
	Chain   Chain        `json:"chain"`
	Status  WalletStatus `json:"status"`
	HasPK   bool         `json:"has_pk"`
	HasSeed bool         `json:"has_seed"`
	// AssociatedHacker marks addresses that transferred funds to a known
	// hacker/drainer operator. It never overrides Status - it is a
	// link-level flag shown next to the status in checks and reports.
	AssociatedHacker bool      `json:"associated_hacker"`
	AssociatedReason string    `json:"associated_reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ContactMessage struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type RateLimit struct {
	ID          int64     `json:"id"`
	IPAddress   string    `json:"ip_address"`
	Count       int       `json:"count"`
	WindowStart time.Time `json:"window_start"`
}

type CheckRequest struct {
	Address string `json:"address" binding:"required"`
	Chain   string `json:"chain" binding:"required"`
}

type CheckResponse struct {
	Address     string `json:"address"`
	Chain       string `json:"chain"`
	Status      string `json:"status"`
	HasPK       bool   `json:"has_pk"`
	HasSeed     bool   `json:"has_seed"`
	Found       bool   `json:"found"`
	BalanceLeft int    `json:"balance_left,omitempty"`
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
	ID            int64      `json:"id"`
	WalletAddress string     `json:"wallet_address"`
	KeyHash       string     `json:"-"`          // SHA-256 hash of the key (never exposed)
	KeyPrefix     string     `json:"key_prefix"` // First 8 chars for identification
	Name          string     `json:"name"`       // User-defined name
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	IsRevoked     bool       `json:"is_revoked"`
}

type CreateAPIKeyRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	ExpiresIn int    `json:"expires_in"` // Days until expiration, 0 = never expires
}

type APIKeyResponse struct {
	Key       string     `json:"key"` // Full key shown only once!
	KeyPrefix string     `json:"key_prefix"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
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
	// Token discount
	SUIDiscountPercent = 50 // 50% discount for SUI token payments

	// SUI token price estimate (mock - should be fetched from oracle)
	SUIUSDPrice = 1.50 // ~$1.50 per SUI

	// API Key settings
	APIKeyLength = 32     // 32 bytes = 64 hex characters
	APIKeyPrefix = "vkn_" // vauln-key prefix for identification
)

// GetPricing returns pricing for different currencies
func GetPricing(checksCount int, pricePerCheck float64) []PaymentMethod {
	basePrice := float64(checksCount) * pricePerCheck

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

// ==================== Admin Models ====================

// AddWalletRequest is the request body for adding wallets
type AddWalletRequest struct {
	SeedPhrase string            `json:"seed_phrase,omitempty"`
	Addresses  map[string]string `json:"addresses"`
	Status     WalletStatus      `json:"status" binding:"required"`
	Reason     string            `json:"reason,omitempty"`
	Source     string            `json:"source,omitempty"`
}

// AddWalletResponse is the response for adding wallets
type AddWalletResponse struct {
	Success        bool            `json:"success"`
	WalletsAdded   int             `json:"wallets_added"`
	WalletsSkipped int             `json:"wallets_skipped"`
	WalletIDs      []int64         `json:"wallet_ids"`
	SkippedWallets []SkippedWallet `json:"skipped_wallets,omitempty"`
	SeedID         *int64          `json:"seed_id,omitempty"`
	SeedSkipped    bool            `json:"seed_skipped,omitempty"`
	Message        string          `json:"message"`
}

// SkippedWallet contains info about skipped duplicate wallet
type SkippedWallet struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
	Reason  string `json:"reason"`
}

// Wallet job statuses for the async import queue
const (
	WalletJobPending    = "pending"
	WalletJobProcessing = "processing"
	WalletJobDone       = "done"
	WalletJobFailed     = "failed"
)

// AddWalletJobResponse describes a queued wallet import job
type AddWalletJobResponse struct {
	JobID     string             `json:"job_id"`
	Status    string             `json:"status"`
	Result    *AddWalletResponse `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// ==================== Reports ====================

// Statuses used inside the report transaction tree. Unlike wallet statuses
// these are derived heuristics, not stored in the wallets table.
const (
	TreeStatusUnknown         = "unknown"
	TreeStatusPotentialHacker = "potential_hacker"
	// TreeStatusProgram marks an on-chain program id that surfaced in the
	// tree instead of the hacker wallet behind it.
	TreeStatusProgram = "program"
)

// AnonymousReportTTL is how long a report stays available for
// non-authenticated users before it is deleted.
const AnonymousReportTTL = 24 * time.Hour

// LeakedKeyInfo describes a leaked credential without exposing its value.
type LeakedKeyInfo struct {
	KeyType      string    `json:"key_type"` // "seed" or "private_key"
	Source       string    `json:"source"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// ReportTxNode is a node of the outgoing transaction tree of a wallet.
type ReportTxNode struct {
	Address          string          `json:"address"`
	TxCount          int             `json:"tx_count"`
	Amount           float64         `json:"amount"`
	Currency         string          `json:"currency"`
	Status           string          `json:"status"`
	AssociatedHacker bool            `json:"associated_hacker,omitempty"`
	IsProgram        bool            `json:"is_program,omitempty"`
	Children         []*ReportTxNode `json:"children,omitempty"`
}

// ReportResponse is the full report payload for a found address.
type ReportResponse struct {
	Address          string           `json:"address"`
	Chain            string           `json:"chain"`
	Found            bool             `json:"found"`
	Status           string           `json:"status"`
	Reason           string           `json:"reason,omitempty"`
	Details          string           `json:"details"`
	Source           string           `json:"source,omitempty"`
	HasPK            bool             `json:"has_pk"`
	HasSeed          bool             `json:"has_seed"`
	AssociatedHacker bool             `json:"associated_hacker,omitempty"`
	AssociatedReason string           `json:"associated_reason,omitempty"`
	Leaks            []LeakedKeyInfo  `json:"leaks,omitempty"`
	Evidence         []StatusEvidence `json:"evidence,omitempty"`
	Transactions     *ReportTxNode    `json:"transactions,omitempty"`
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
	Public           bool             `json:"public,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}

// ShareReportRequest asks to make a report public (authenticated users only).
type ShareReportRequest struct {
	Address string `json:"address" binding:"required"`
	Chain   string `json:"chain" binding:"required"`
}

// ShareReportResponse carries the public share link of a report.
type ShareReportResponse struct {
	ShareID  string `json:"share_id"`
	ShareURL string `json:"share_url"` // frontend path, e.g. /report/<uuid>
}

// ==================== Drainer scanner (solana_scan.py) ====================

// Scanner verdicts produced by solana_scan.py.
const (
	ScanVerdictDrainer    = "DRAINER"
	ScanVerdictSuspicious = "SUSPICIOUS"
)

// ScanFinding is one drainer-pattern detection reported by solana_scan.py.
// VictimAddress is the drained/hijacked wallet, HackerAddress is the sweep
// destination or the program that took over the account.
type ScanFinding struct {
	ID            int64     `json:"id"`
	Chain         string    `json:"chain"`
	Signature     string    `json:"signature"`
	Slot          int64     `json:"slot"`
	Verdict       string    `json:"verdict"`
	Indicators    []string  `json:"indicators"`
	VictimAddress string    `json:"victim_address,omitempty"`
	HackerAddress string    `json:"hacker_address,omitempty"`
	AmountSOL     float64   `json:"amount_sol"`
	Programs      []string  `json:"programs,omitempty"`
	Source        string    `json:"source"` // scanner mode: watch / scan-wallet
	CreatedAt     time.Time `json:"created_at"`
}

// ScanFindingRequest is the ingest payload sent by solana_scan.py.
type ScanFindingRequest struct {
	Chain         string   `json:"chain"`
	Signature     string   `json:"signature" binding:"required"`
	Slot          int64    `json:"slot"`
	Verdict       string   `json:"verdict" binding:"required"`
	Indicators    []string `json:"indicators"`
	VictimAddress string   `json:"victim_address"`
	HackerAddress string   `json:"hacker_address"`
	AmountSOL     float64  `json:"amount_sol"`
	Programs      []string `json:"programs"`
	Source        string   `json:"source"`
	// ExposedAddresses are wallets that sent funds to the hacker in this
	// drainer transaction. They get flagged as associated with a hacker
	// without their status being escalated automatically.
	ExposedAddresses []string `json:"exposed_addresses"`
}

// ScanStats are aggregate counters for the live monitoring page.
type ScanStats struct {
	TotalFindings int64   `json:"total_findings"`
	DrainerCount  int64   `json:"drainer_count"`
	SuspectCount  int64   `json:"suspicious_count"`
	VictimCount   int64   `json:"victim_count"`
	HackerCount   int64   `json:"hacker_count"`
	StolenSOL     float64 `json:"stolen_sol"`
}

// ==================== User drainer reports ====================

// DrainerReport is a user-submitted report about a drainer theft.
type DrainerReport struct {
	ID           int64     `json:"id"`
	TxSignature  string    `json:"tx_signature"`
	Chain        string    `json:"chain"`
	SiteURL      string    `json:"site_url,omitempty"`
	Description  string    `json:"description,omitempty"`
	Reporter     string    `json:"-"` // wallet address or "ip:<ip>"
	Status       string    `json:"status"`
	TelegramSent bool      `json:"telegram_sent"`
	CreatedAt    time.Time `json:"created_at"`
}

// DrainerReportRequest is the public submission payload. A valid captcha
// answer is required to protect the endpoint from bots.
type DrainerReportRequest struct {
	TxSignature   string `json:"tx_signature" binding:"required,min=40,max=120"`
	Chain         string `json:"chain"`
	SiteURL       string `json:"site_url" binding:"omitempty,max=300"`
	Description   string `json:"description" binding:"omitempty,max=2000"`
	CaptchaID     string `json:"captcha_id" binding:"required"`
	CaptchaAnswer string `json:"captcha_answer" binding:"required"`
}

// CaptchaResponse carries a one-time captcha challenge for the report form.
type CaptchaResponse struct {
	CaptchaID string `json:"captcha_id"`
	Image     string `json:"image"` // data URI with the captcha SVG
}

// ==================== Status evidence chain ====================

// StatusEvidence is one step of the chain explaining why a wallet has its
// status: registry listing, key leak, or a scanner indicator (P1..P5).
type StatusEvidence struct {
	Code         string    `json:"code"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	TxSignature  string    `json:"tx_signature,omitempty"`
	Counterparty string    `json:"counterparty,omitempty"`
	AmountSOL    float64   `json:"amount_sol,omitempty"`
	DetectedAt   time.Time `json:"detected_at,omitempty"`
}
