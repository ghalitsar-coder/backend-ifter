package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

var ErrInvalidToken = errors.New("invalid token")

// Claims is the access-token payload. Roles and school are embedded so
// authorization checks do not need a database round trip on every request.
type Claims struct {
	SchoolID int64    `json:"school_id"`
	Roles    []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
}

func NewTokenService(cfg config.AuthConfig) *TokenService {
	return &TokenService{
		secret:   []byte(cfg.JWTSecret),
		issuer:   cfg.JWTIssuer,
		audience: cfg.JWTAudience,
		ttl:      cfg.AccessTokenTTL,
	}
}

func (s *TokenService) Issue(subject string, roles []string, schoolID int64) (string, error) {
	if subject == "" {
		return "", errors.New("token subject is required")
	}
	if schoolID <= 0 {
		return "", errors.New("token school is required")
	}
	now := time.Now()
	claims := Claims{
		SchoolID: schoolID,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    s.issuer,
			Audience:  claimAudience(s.audience),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
			ID:        uuid.NewString(),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

func (s *TokenService) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30 * time.Second),
	}
	if s.audience != "" {
		opts = append(opts, jwt.WithAudience(s.audience))
	}

	if _, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return s.secret, nil
	}, opts...); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}
	if claims.SchoolID <= 0 {
		return nil, fmt.Errorf("%w: missing school", ErrInvalidToken)
	}
	return claims, nil
}

func claimAudience(audience string) jwt.ClaimStrings {
	if audience == "" {
		return nil
	}
	return jwt.ClaimStrings{audience}
}
