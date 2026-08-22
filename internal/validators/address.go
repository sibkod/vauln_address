package validators

import (
	"math/big"
	"regexp"
	"strings"

	"vauln-address/internal/models"
)

var (
	// EVM: 0x + 40 hex characters (42 total)
	evmRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)

	// Bitcoin: 26-35 chars, starts with 1, 3, or bc1
	btcRegexLegacy = regexp.MustCompile(`^1[a-km-zA-HJ-NP-Z1-9]{25,34}$`)
	btcRegexScript = regexp.MustCompile(`^3[a-km-zA-HJ-NP-Z1-9]{25,34}$`)
	btcRegexBech32 = regexp.MustCompile(`^bc1[a-zA-HJ-NP-Z0-9]{25,89}$`)

	// Solana: 32-44 chars, base58
	solanaRegex = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)

	// Sui: 0x + 64 hex characters (66 total)
	suiRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{64}$`)

	// Tron: 34 chars, starts with T
	tronRegex = regexp.MustCompile(`^T[a-km-zA-HJ-NP-Z1-9]{33}$`)
)

func ValidateAddress(chain, address string) (bool, string) {
	chain = strings.ToLower(chain)
	address = strings.TrimSpace(address)

	if address == "" {
		return false, "address is required"
	}

	switch models.Chain(chain) {
	case models.ChainEVM:
		return validateEVM(address)
	case models.ChainBTC:
		return validateBTC(address)
	case models.ChainSolana:
		return validateSolana(address)
	case models.ChainSui:
		return validateSui(address)
	case models.ChainTron:
		return validateTron(address)
	default:
		return false, "unsupported chain"
	}
}

func validateEVM(address string) (bool, string) {
	if evmRegex.MatchString(address) {
		return true, ""
	}
	return false, "invalid EVM address format: must be 0x followed by 40 hex characters"
}

func validateBTC(address string) (bool, string) {
	if btcRegexLegacy.MatchString(address) ||
		btcRegexScript.MatchString(address) ||
		btcRegexBech32.MatchString(address) {
		return true, ""
	}
	return false, "invalid Bitcoin address format: must be 26-35 chars starting with 1, 3, or bc1"
}

func validateSolana(address string) (bool, string) {
	if !solanaRegex.MatchString(address) || len(address) < 32 || len(address) > 44 {
		return false, "invalid Solana address format: must be 32-44 base58 characters"
	}
	if !solanaDecodesTo32Bytes(address) {
		return false, "invalid Solana address: does not decode to a 32-byte public key"
	}
	return true, ""
}

// solanaDecodesTo32Bytes checks that a base58 string is a well-formed
// encoding of exactly 32 bytes — the length of a Solana public key.
// Arbitrary base58 strings with leading-zero padding or oversized payloads
// pass the character regex but are not real addresses.
func solanaDecodesTo32Bytes(address string) bool {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	value := big.NewInt(0)
	base := big.NewInt(58)
	for _, ch := range address {
		idx := strings.IndexRune(alphabet, ch)
		if idx < 0 {
			return false
		}
		value.Mul(value, base)
		value.Add(value, big.NewInt(int64(idx)))
	}
	body := value.Bytes()
	// ведущие '1' — это нулевые байты; остальное — само число
	leading := 0
	for leading < len(address) && address[leading] == '1' {
		leading++
	}
	return leading+len(body) == 32
}

func validateSui(address string) (bool, string) {
	if suiRegex.MatchString(address) {
		return true, ""
	}
	return false, "invalid Sui address format: must be 0x followed by 64 hex characters"
}

func validateTron(address string) (bool, string) {
	if tronRegex.MatchString(address) {
		return true, ""
	}
	return false, "invalid Tron address format: must be 34 characters starting with T"
}
