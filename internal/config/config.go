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
	DBType            DBType
	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBCharset         string
	SQLitePath        string
	ServerPort        string
	RateLimitRequests int
	RateLimitHours    int
	FreeCheckLimit    int
	SolanaPaymentAddr string
	SolanaRPCURL      string
	SolanaUseDevnet   bool
	AdminAPIKey       string
}

func Load() *Config {
	godotenv.Load()

	dbType := DBType(getEnv("DB_TYPE", "postgres"))
	
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
		DBType:            dbType,
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnvInt("DB_PORT", 5432),
		DBUser:            getEnv("DB_USER", "postgres"),
		DBPassword:        getEnv("DB_PASSWORD", "postgres"),
		DBName:            getEnv("DB_NAME", "vauln_address"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		DBCharset:         getEnv("DB_CHARSET", "utf8mb4"),
		SQLitePath:        getEnv("SQLITE_PATH", "./data/vauln_address.db"),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitHours:    getEnvInt("RATE_LIMIT_WINDOW_HOURS", 24),
		FreeCheckLimit:    getEnvInt("FREE_CHECK_LIMIT", 1),
		SolanaPaymentAddr: getEnv("SOLANA_PAYMENT_ADDR", ""),
		SolanaRPCURL:      solanaRPCURL,
		SolanaUseDevnet:   solanaUseDevnet,
		AdminAPIKey:       getEnv("ADMIN_API_KEY", ""),
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
