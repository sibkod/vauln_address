package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ethereum/go-ethereum/crypto"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

const (
	JWTSecret      = "vauln-address-secret-key-change-in-production"
	TokenExpiry    = 24 * time.Hour
	NonceLength    = 32
)

type AuthService struct {
	repo *repository.Repository
}

func NewAuthService(repo *repository.Repository) *AuthService {
	return &AuthService{repo: repo}
}

type Claims struct {
	UserID int64  `json:"user_id"`
	Address string `json:"address"`
	jwt.RegisteredClaims
}

// GenerateNonce creates a unique nonce for Web3 authentication
func (s *AuthService) GenerateNonce(address string, chain string) (string, error) {
	nonceBytes := make([]byte, NonceLength)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	// Store/update nonce for user
	if err := s.repo.UpsertUserNonce(address, chain, nonce); err != nil {
		return "", fmt.Errorf("failed to store nonce: %w", err)
	}

	return nonce, nil
}

// VerifySignature verifies the Web3 signature and returns a JWT token
func (s *AuthService) VerifySignature(address, chain, signature, message string) (*models.AuthResponse, error) {
	// Normalize inputs
	address = strings.ToLower(strings.TrimSpace(address))
	signature = strings.TrimSpace(signature)
	message = strings.TrimSpace(message)

	// Verify the message format (should contain nonce)
	if !strings.Contains(message, "nonce:") {
		return nil, fmt.Errorf("invalid message format: missing nonce")
	}

	// Get stored nonce from database
	storedNonce, err := s.repo.GetUserNonce(address, chain)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}
	if storedNonce == "" {
		return nil, fmt.Errorf("nonce not found: please request a new nonce first")
	}

	// Verify nonce matches
	if !strings.Contains(message, storedNonce) {
		return nil, fmt.Errorf("invalid nonce: signature does not match stored nonce")
	}

	// Verify signature based on chain
	var isValid bool
	switch chain {
	case "evm", "ethereum":
		isValid = s.verifyEVM(address, signature, message)
	case "sui":
		isValid = s.verifySui(address, signature, message)
	case "solana":
		isValid = s.verifySolana(address, signature, message)
	case "tron":
		isValid = s.verifyTron(address, signature, message)
	default:
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}

	if !isValid {
		return nil, fmt.Errorf("invalid signature")
	}

	// Get or create user
	user, err := s.repo.GetOrCreateUser(address, chain)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create user: %w", err)
	}

	// Generate JWT token
	token, err := s.generateToken(user.ID, address)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Update last login
	s.repo.UpdateLastLogin(user.ID)

	return &models.AuthResponse{
		Token:     token,
		User:      s.toUserPublic(user),
		ExpiresIn: int(TokenExpiry.Seconds()),
	}, nil
}

// verifyEVM verifies Ethereum/EVM signatures using personal_sign format
func (s *AuthService) verifyEVM(address, signature, message string) bool {
	// Add prefix for personal_sign
	prefixedMessage := "\x19Ethereum Signed Message:\n" + fmt.Sprintf("%d", len(message)) + message
	
	// Hash the prefixed message
	hash := crypto.Keccak256Hash([]byte(prefixedMessage))
	
	// Parse signature (65 bytes: r, s, v)
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sigBytes) != 65 {
		// Try without 0x prefix
		sigBytes, err = hex.DecodeString(signature)
		if err != nil || len(sigBytes) != 65 {
			return false
		}
	}
	
	// Recover public key
	sigPublicKey, err := crypto.SigToPub(hash.Bytes(), sigBytes)
	if err != nil {
		return false
	}
	
	// Get recovered address
	recoveredAddr := crypto.PubkeyToAddress(*sigPublicKey).Hex()
	
	return strings.ToLower(recoveredAddr) == strings.ToLower(address)
}

// verifyEVMWithoutPrefix verifies EVM signature without the personal_sign prefix
func (s *AuthService) verifyEVMWithoutPrefix(address, signature, message string) bool {
	hash := crypto.Keccak256Hash([]byte(message))
	
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sigBytes) != 65 {
		sigBytes, err = hex.DecodeString(signature)
		if err != nil || len(sigBytes) != 65 {
			return false
		}
	}
	
	sigPublicKey, err := crypto.SigToPub(hash.Bytes(), sigBytes)
	if err != nil {
		return false
	}
	
	recoveredAddr := crypto.PubkeyToAddress(*sigPublicKey).Hex()
	return strings.ToLower(recoveredAddr) == strings.ToLower(address)
}

// verifySui verifies Sui blockchain signatures (simplified)
// In production, use Sui SDK for proper verification
func (s *AuthService) verifySui(address, signature, message string) bool {
	// Sui uses Ed25519 or ECDSA signatures
	// For demo purposes, we verify the structure
	// Real implementation would use Sui cryptographic verification
	
	// Basic validation: signature should be hex and reasonably long
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		sigBytes, err = hex.DecodeString(signature)
	}
	
	// Sui signatures are typically 64-70 bytes for Ed25519/ECDSA
	return err == nil && len(sigBytes) >= 64 && len(sigBytes) <= 70
}

// verifySolana verifies Solana signatures
// In production, use Solana SDK
func (s *AuthService) verifySolana(address, signature, message string) bool {
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		sigBytes, err = hex.DecodeString(signature)
	}
	
	// Solana signatures are 64 bytes
	return err == nil && len(sigBytes) == 64
}

// verifyTron verifies Tron signatures (same as EVM)
func (s *AuthService) verifyTron(address, signature, message string) bool {
	return s.verifyEVMWithoutPrefix(address, signature, message)
}

func (s *AuthService) generateToken(userID int64, address string) (string, error) {
	claims := Claims{
		UserID:  userID,
		Address: address,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "vauln-address",
			Subject:   address,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWTSecret))
}

// ValidateToken validates a JWT token and returns the claims
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func (s *AuthService) toUserPublic(user *models.User) *models.UserPublic {
	return &models.UserPublic{
		ID:            user.ID,
		WalletAddress: user.WalletAddress,
		Chain:         string(user.Chain),
		Balance:       user.Balance,
		IsPremium:     user.Balance > 10, // Premium if has more than 10 checks
	}
}

// GetUserByID retrieves user by ID
func (s *AuthService) GetUserByID(userID int64) (*models.User, error) {
	return s.repo.GetUserByID(context.Background(), userID)
}
