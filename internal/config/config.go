package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// DBType represents the type of database
type DBType string

const (
	DBTypePostgres DBType = "postgres"
	DBTypeMySQL    DBType = "mysql"
	DBTypeSQLite   DBType = "sqlite"
)

type Config struct {
	DBType       DBType
	DBHost       string
	DBPort       int
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	DBCharset    string
	DBUnixSocket string
	SQLitePath   string
	// Connection pool (0 = built-in defaults)
	DBMaxOpenConns       int
	DBMaxIdleConns       int
	DBConnMaxLifetimeMin int
	ServerPort           string
	RateLimitRequests    int
	RateLimitHours       int
	FreeCheckLimit       int
	SolanaPaymentAddr    string
	SolanaRPCURL         string
	SolanaUseDevnet      bool
	AdminAPIKey          string
	// Price settings
	SolanaPriceUSD   float64
	PricePerCheckUSD float64
	// Wallet import queue settings
	WalletQueueSize       int
	WalletBatchSize       int
	WalletFlushIntervalMs int
	WalletSyncWaitSeconds int
	// Telegram bot for drainer report notifications
	TelegramBotToken string
	TelegramChatID   string
}

func Load() *Config {
	godotenv.Load()

	dbType := DBType(getEnv("DB_TYPE", "mysql"))

	// Solana configuration - easy 1-line switch between devnet and mainnet
	// Set SOLANA_USE_DEVNET=false for mainnet
	solanaUseDevnet := getEnv("SOLANA_USE_DEVNET", "true") == "true"

	// Default RPC URLs
	solanaRPCURL := getEnv("SOLANA_RPC_URL", "")
	if solanaRPCURL == "" {
		if solanaUseDevnet {
			solanaRPCURL = "https://api.devnet.solana.com"
		} else {
			solanaRPCURL = "https://api.mainnet-beta.solana.com"
		}
	}

	return &Config{
		DBType:                dbType,
		DBHost:                getEnv("DB_HOST", "localhost"),
		DBPort:                getEnvInt("DB_PORT", 5432),
		DBUser:                getEnv("DB_USER", "postgres"),
		DBPassword:            getEnv("DB_PASSWORD", "postgres"),
		DBName:                getEnv("DB_NAME", "vauln_address"),
		DBSSLMode:             getEnv("DB_SSLMODE", "disable"),
		DBCharset:             getEnv("DB_CHARSET", "utf8mb4"),
		DBUnixSocket:          getEnv("DB_UNIX_SOCKET", ""),
		SQLitePath:            getEnv("SQLITE_PATH", "./data/vauln_address.db"),
		DBMaxOpenConns:        getEnvInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:        getEnvInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetimeMin:  getEnvInt("DB_CONN_MAX_LIFETIME_MIN", 5),
		ServerPort:            getEnv("SERVER_PORT", "9111"),
		RateLimitRequests:     getEnvInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitHours:        getEnvInt("RATE_LIMIT_WINDOW_HOURS", 24),
		FreeCheckLimit:        getEnvInt("FREE_CHECK_LIMIT", 3),
		SolanaPaymentAddr:     getEnv("SOLANA_PAYMENT_ADDR", ""),
		SolanaRPCURL:          solanaRPCURL,
		SolanaUseDevnet:       solanaUseDevnet,
		AdminAPIKey:           getEnv("ADMIN_API_KEY", ""),
		SolanaPriceUSD:        getEnvFloat("SOLANA_PRICE_USD", 150.0),
		PricePerCheckUSD:      getEnvFloat("PRICE_PER_CHECK_USD", 0.10),
		WalletQueueSize:       getEnvInt("WALLET_QUEUE_SIZE", 10000),
		WalletBatchSize:       getEnvInt("WALLET_BATCH_SIZE", 100),
		WalletFlushIntervalMs: getEnvInt("WALLET_FLUSH_INTERVAL_MS", 200),
		WalletSyncWaitSeconds: getEnvInt("WALLET_SYNC_WAIT_SECONDS", 10),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:        getEnv("TELEGRAM_CHAT_ID", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
