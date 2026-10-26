package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrRefreshReuse       = errors.New("refresh token reuse detected")
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrLastAdmin          = errors.New("last active administrator")
	ErrUserNotReady       = errors.New("user has not completed activation")
	ErrInvalidCredential  = errors.New("invalid credential token")
)

// dummyHash keeps login timing close to a real bcrypt check when the account
// does not exist. It is a bcrypt hash of a value that cannot be a real login.
const dummyHash = "$2a$12$hKB4hJ2BuL0rIv0enn4.meowk8Nyl10iGSGO8ulNJZn.ioLGHRzmG"

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email,omitempty"`
	FullName string   `json:"full_name"`
	Roles    []string `json:"roles"`
}

type UserSummary struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email,omitempty"`
	FullName    string     `json:"full_name"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type UserAccount struct {
	UserSummary
	Roles []string `json:"roles"`
}

type Role struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CreateUserInput struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Roles    []string `json:"roles"`
}

type CredentialIssue struct {
	Token     string    `json:"token"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Session struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             User
}

type BootstrapAdmin struct {
	Username string
	Email    string
	Password string
	FullName string
}

type ServiceConfig struct {
	RefreshTokenTTL time.Duration
	InviteTokenTTL  time.Duration
	ResetTokenTTL   time.Duration
	BcryptCost      int
}

type TokenManager struct {
	// secrets[0] signs new tokens; the rest are accepted for verification so a
	// secret can be rotated without invalidating live access tokens.
	secrets   [][]byte
	issuer    string
	accessTTL time.Duration
}

// accessTokenClaims carries the user's token_version so Authenticate can
// reject tokens issued before a password reset.
type accessTokenClaims struct {
	Version int `json:"ver"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret, issuer string, accessTTL time.Duration, previousSecrets ...string) *TokenManager {
	secrets := make([][]byte, 0, 1+len(previousSecrets))
	secrets = append(secrets, []byte(secret))
	for _, previous := range previousSecrets {
		if previous = strings.TrimSpace(previous); previous != "" {
			secrets = append(secrets, []byte(previous))
		}
	}
	return &TokenManager{secrets: secrets, issuer: issuer, accessTTL: accessTTL}
}

func (m *TokenManager) Issue(userID string, tokenVersion int) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.accessTTL)
	claims := accessTokenClaims{
		Version: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secrets[0])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) Parse(raw string) (string, int, error) {
	for _, secret := range m.secrets {
		claims := &accessTokenClaims{}
		token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrUnauthorized
			}
			return secret, nil
		},
			jwt.WithIssuer(m.issuer),
			jwt.WithExpirationRequired(),
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithLeeway(30*time.Second),
		)
		if err == nil && token.Valid && claims.Subject != "" {
			return claims.Subject, claims.Version, nil
		}
	}
	return "", 0, ErrUnauthorized
}

func newOpaqueToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate opaque token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashOpaqueToken(raw), nil
}

func hashOpaqueToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hashPassword(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
