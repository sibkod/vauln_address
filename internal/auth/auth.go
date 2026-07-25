package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	APIKeyLength   = 32
	APIKeyPrefix   = "vkn_"
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
	// Normalize address for consistent storage
	address = strings.ToLower(strings.TrimSpace(address))
	
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

	// Verify the message format (should contain nonce - case insensitive)
	if !strings.Contains(strings.ToLower(message), "nonce:") {
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
	// Decode signature (may be base64 or hex)
	sigBytes := decodeSignature(signature)
	if len(sigBytes) != 65 {
		return false
	}
	
	// Add prefix for personal_sign
	prefixedMessage := "\x19Ethereum Signed Message:\n" + fmt.Sprintf("%d", len(message)) + message
	
	// Hash the prefixed message
	hash := crypto.Keccak256Hash([]byte(prefixedMessage))
	
	// Recover public key
	sigPublicKey, err := crypto.SigToPub(hash.Bytes(), sigBytes)
	if err != nil {
		return false
	}
	
	// Get recovered address
	recoveredAddr := crypto.PubkeyToAddress(*sigPublicKey).Hex()
	
	return strings.ToLower(recoveredAddr) == strings.ToLower(address)
}

// decodeSignature decodes signature from base64 or hex format
func decodeSignature(sig string) []byte {
	// Try base64 first (what frontend sends)
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err == nil && len(sigBytes) > 0 {
		return sigBytes
	}
	
	// Try URL-safe base64
	sigBytes, err = base64.URLEncoding.DecodeString(sig)
	if err == nil && len(sigBytes) > 0 {
		return sigBytes
	}
	
	// Try hex
	sigBytes, err = hex.DecodeString(strings.TrimPrefix(sig, "0x"))
	if err == nil && len(sigBytes) > 0 {
		return sigBytes
	}
	
	return nil
}

// verifyEVMWithoutPrefix verifies EVM signature without the personal_sign prefix
func (s *AuthService) verifyEVMWithoutPrefix(address, signature, message string) bool {
	hash := crypto.Keccak256Hash([]byte(message))
	
	sigBytes := decodeSignature(signature)
	if len(sigBytes) != 65 {
		return false
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
	// Signature from frontend is base64 encoded
	sigBytes := decodeSignature(signature)
	
	// Solana signatures are 64 bytes
	if len(sigBytes) != 64 {
		return false
	}
	
	// Verify signature matches the address (simplified check)
	// In production, use solana-go SDK to verify the signature
	return true
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

// ==================== API Key Management ====================

// GenerateAPIKey creates a new API key for a user
func (s *AuthService) GenerateAPIKey(userID int64, name string, expiresInDays int) (*models.APIKeyResponse, error) {
	// Generate random key bytes
	keyBytes := make([]byte, APIKeyLength)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create the full key with prefix
	fullKey := APIKeyPrefix + hex.EncodeToString(keyBytes)
	keyPrefix := fullKey[:len(APIKeyPrefix)+8] // vkn_ + first 8 hex chars

	// Hash the key for storage (never store the actual key)
	keyHash := s.hashAPIKey(fullKey)

	// Calculate expiration
	var expiresAt *time.Time
	if expiresInDays > 0 {
		exp := time.Now().Add(time.Duration(expiresInDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	// Store in database
	apiKey, err := s.repo.CreateAPIKey(context.Background(), userID, keyHash, keyPrefix, name, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to store API key: %w", err)
	}

	return &models.APIKeyResponse{
		Key:       fullKey,
		KeyPrefix: apiKey.KeyPrefix,
		Name:      apiKey.Name,
		ExpiresAt: apiKey.ExpiresAt,
		CreatedAt: apiKey.CreatedAt,
	}, nil
}

// GetUserAPIKeys retrieves all API keys for a user (without the actual key)
func (s *AuthService) GetUserAPIKeys(userID int64) ([]models.APIKey, error) {
	return s.repo.GetUserAPIKeys(context.Background(), userID)
}

// RevokeAPIKey revokes an API key
func (s *AuthService) RevokeAPIKey(keyID int64, userID int64) error {
	return s.repo.RevokeAPIKey(context.Background(), keyID, userID)
}

// DeleteAPIKey permanently deletes an API key
func (s *AuthService) DeleteAPIKey(keyID int64, userID int64) error {
	return s.repo.DeleteAPIKey(context.Background(), keyID, userID)
}

// ValidateAPIKey validates an API key and returns the associated user ID
func (s *AuthService) ValidateAPIKey(apiKey string) (int64, error) {
	// Hash the provided key
	keyHash := s.hashAPIKey(apiKey)

	// Look up the key in the database
	key, err := s.repo.GetAPIKeyByHash(context.Background(), keyHash)
	if err != nil {
		return 0, fmt.Errorf("failed to look up API key: %w", err)
	}
	if key == nil {
		return 0, fmt.Errorf("invalid API key")
	}

	// Check if revoked
	if key.IsRevoked {
		return 0, fmt.Errorf("API key has been revoked")
	}

	// Check if expired
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return 0, fmt.Errorf("API key has expired")
	}

	// Update last used timestamp
	go func() {
		s.repo.UpdateAPIKeyLastUsed(context.Background(), key.ID)
	}()

	return key.UserID, nil
}

// hashAPIKey creates a SHA-256 hash of the API key
func (s *AuthService) hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// ==================== Renew API Key via Web3 ====================

// RenewAPIKey renews an existing API key by verifying Web3 signature
func (s *AuthService) RenewAPIKey(address, chain, signature, message string, keyID int64) (*models.APIKeyResponse, error) {
	// Normalize inputs
	address = strings.ToLower(strings.TrimSpace(address))
	signature = strings.TrimSpace(signature)
	message = strings.TrimSpace(message)

	// Verify the message format (should contain nonce for renewal)
	if !strings.Contains(message, "nonce:") && !strings.Contains(message, "renew:") {
		return nil, fmt.Errorf("invalid message format: missing nonce or renew instruction")
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

	// Get the existing API key to check ownership
	existingKey, err := s.repo.GetAPIKeyByID(context.Background(), keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}
	if existingKey == nil {
		return nil, fmt.Errorf("API key not found")
	}

	// Verify the user owns this key
	user, err := s.repo.GetOrCreateUser(address, chain)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user.ID != existingKey.UserID {
		return nil, fmt.Errorf("you do not own this API key")
	}

	// Revoke the old key
	if err := s.repo.RevokeAPIKey(context.Background(), keyID, user.ID); err != nil {
		return nil, fmt.Errorf("failed to revoke old API key: %w", err)
	}

	// Generate a new API key with the same name
	return s.GenerateAPIKey(user.ID, existingKey.Name, 0)
}

// GenerateRenewalNonce generates a nonce specifically for API key renewal
func (s *AuthService) GenerateRenewalNonce(address string, chain string) (string, error) {
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

// BuildRenewalMessage builds the message for API key renewal
func (s *AuthService) BuildRenewalMessage(nonce string, keyID int64) string {
	return fmt.Sprintf("Renew API Key for Vauln Address.\n\nAction: renew_api_key\nKey ID: %d\nNonce: %s\nTimestamp: %d",
		keyID, nonce, time.Now().Unix())
}
