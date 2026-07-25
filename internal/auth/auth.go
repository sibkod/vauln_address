package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

const (
	JWTSecret   = "vauln-address-secret-key-change-in-production"
	TokenExpiry = 24 * time.Hour
)

type AuthService struct {
	repo *repository.Repository
}

func NewAuthService(repo *repository.Repository) *AuthService {
	return &AuthService{repo: repo}
}

type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Register creates a new user with email and password
func (s *AuthService) Register(email, password string) (*models.AuthResponse, error) {
	ctx := context.Background()
	
	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))
	
	// Check if user already exists
	existingUser, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return nil, fmt.Errorf("user with this email already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user, err := s.repo.CreateUser(ctx, email, string(hashedPassword))
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate JWT token
	token, err := s.generateToken(user.ID, email)
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

// Login authenticates a user with email and password
func (s *AuthService) Login(email, password string) (*models.AuthResponse, error) {
	ctx := context.Background()
	
	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))
	
	// Get user by email
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Generate JWT token
	token, err := s.generateToken(user.ID, email)
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

func (s *AuthService) generateToken(userID int64, email string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "vauln-address",
			Subject:   email,
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
		ID:        user.ID,
		Email:     user.Email,
		Balance:   user.Balance,
		IsPremium: user.IsPremium,
	}
}

// GetUserByID retrieves user by ID
func (s *AuthService) GetUserByID(userID int64) (*models.User, error) {
	return s.repo.GetUserByID(context.Background(), userID)
}
