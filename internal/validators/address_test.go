package validators

import "testing"

func TestValidateEVMAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"valid address", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", true},
		{"valid lowercase", "0xabcdef1234567890abcdef1234567890abcdef12", true},
		{"valid uppercase", "0xABCDEF1234567890ABCDEF1234567890ABCDEF12", true},
		{"valid mixed", "0xAbCdEf1234567890AbCdEf1234567890AbCdEf12", true},
		{"too short", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B", false},
		{"too long", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1aa", false},
		{"no 0x prefix", "742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", false},
		{"invalid chars", "0x742d35Cc6634C0532925a3b844Bc9e7595f5G2a1", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validateEVM(tt.address)
			if valid != tt.want {
				t.Errorf("validateEVM(%q) = %v, want %v", tt.address, valid, tt.want)
			}
		})
	}
}

func TestValidateBTCAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"valid legacy 1", "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", true},
		{"valid legacy 3", "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", true},
		{"valid bech32", "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", true},
		{"valid bech32 long", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", true},
		{"too short", "1BvBMSEYstWetqTFn5Au", false},
		{"too long", "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2xxx", false},
		{"wrong prefix", "2BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", false},
		{"invalid char", "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVNz", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validateBTC(tt.address)
			if valid != tt.want {
				t.Errorf("validateBTC(%q) = %v, want %v", tt.address, valid, tt.want)
			}
		})
	}
}

func TestValidateSolanaAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"valid address", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV", true},
		{"valid short", "1111111111111111", true},
		{"too short", "7EcDhSYGxXys", false},
		{"too long", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtVxxx", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validateSolana(tt.address)
			if valid != tt.want {
				t.Errorf("validateSolana(%q) = %v, want %v", tt.address, valid, tt.want)
			}
		})
	}
}

func TestValidateSuiAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"valid address", "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6", true},
		{"valid zeros", "0x0000000000000000000000000000000000000000000000000000000000000000", true},
		{"valid ff", "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", true},
		{"no 0x prefix", "8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6", false},
		{"too short", "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4", false},
		{"too long", "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6aa", false},
		{"invalid char", "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4g6", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validateSui(tt.address)
			if valid != tt.want {
				t.Errorf("validateSui(%q) = %v, want %v", tt.address, valid, tt.want)
			}
		})
	}
}

func TestValidateTronAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{"valid address", "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd", true},
		{"valid T", "TJygL3D2K8M7fGhJkLmNpQrStUvWxYzAbCd", true},
		{"wrong prefix", "AJ5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd", false},
		{"too short", "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3b", false},
		{"too long", "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCdaa", false},
		{"invalid char", "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validateTron(tt.address)
			if valid != tt.want {
				t.Errorf("validateTron(%q) = %v, want %v", tt.address, valid, tt.want)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		chain   string
		address string
		want    bool
	}{
		{"EVM valid", "evm", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", true},
		{"BTC valid", "btc", "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", true},
		{"Solana valid", "solana", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV", true},
		{"Sui valid", "sui", "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6", true},
		{"Tron valid", "tron", "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd", true},
		{"unsupported chain", "ethereum", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", false},
		{"empty address", "evm", "", false},
		{"case insensitive chain", "EVM", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := ValidateAddress(tt.chain, tt.address)
			if valid != tt.want {
				t.Errorf("ValidateAddress(%q, %q) = %v, want %v", tt.chain, tt.address, valid, tt.want)
			}
		})
	}
}
