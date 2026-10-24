package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrNoRole             = errors.New("account has no role")
)

// User is the stored identity needed to verify a password and mint a token.
// Roles always come from the database, never from the request.
type User struct {
	ID           int64
	SchoolID     int64
	Username     string
	PasswordHash string
	IsActive     bool
	Nama         string
	Roles        []string
}

// LoginResult is the access token plus the profile the client shows after a
// successful login.
type LoginResult struct {
	Token     string   `json:"token"`
	ExpiresAt string   `json:"expires_at"`
	UserID    int64    `json:"user_id"`
	Username  string   `json:"username"`
	Nama      string   `json:"nama"`
	Roles     []string `json:"roles"`
}

type LoginService struct {
	repo   Repository
	tokens *TokenService
	ttl    time.Duration
}

func NewLoginService(repo Repository, tokens *TokenService, ttl time.Duration) *LoginService {
	return &LoginService{repo: repo, tokens: tokens, ttl: ttl}
}

// Login verifies the password against the stored bcrypt hash and issues a
// token whose subject is the users.id. Unknown username and wrong password
// return the same error so accounts cannot be enumerated. Whether the user has
// a staff row is not checked here: the attendance service reports that.
func (s *LoginService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	// Registered usernames are emails, so case must not create a second identity.
	user, err := s.repo.FindByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		if database.IsNotFound(err) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("find user by username: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if !user.IsActive {
		return LoginResult{}, ErrAccountDisabled
	}
	if len(user.Roles) == 0 {
		return LoginResult{}, ErrNoRole
	}

	token, err := s.tokens.Issue(strconv.FormatInt(user.ID, 10), user.Roles, user.SchoolID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue access token: %w", err)
	}
	return LoginResult{
		Token:     token,
		ExpiresAt: time.Now().Add(s.ttl).UTC().Format(time.RFC3339),
		UserID:    user.ID,
		Username:  user.Username,
		Nama:      user.Nama,
		Roles:     user.Roles,
	}, nil
}
