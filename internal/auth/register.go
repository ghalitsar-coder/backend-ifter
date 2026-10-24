package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

var (
	ErrInvalidRegistration = errors.New("invalid registration data")
	ErrEmailTaken          = errors.New("email is already registered")
	ErrNPSNTaken           = errors.New("npsn is already registered")
)

var (
	npsnPattern  = regexp.MustCompile(`^[0-9]{8}$`)
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// RegisterInput is the school onboarding payload. Documents and the OTP are
// still client-side demo steps, so they are not part of the backend contract.
type RegisterInput struct {
	SchoolName string
	NPSN       string
	Jenjang    string
	AdminName  string
	Email      string
	Phone      string
	Password   string
}

type RegisterResult struct {
	UserID   int64
	SchoolID int64
	Username string
}

// RegisterRepository is separate from Repository so login fakes stay small.
type RegisterRepository interface {
	RegisterSchoolAdmin(ctx context.Context, in RegisterInput, username, passwordHash string) (RegisterResult, error)
}

type RegisterService struct {
	repo   RegisterRepository
	tokens *TokenService
	ttl    time.Duration
}

func NewRegisterService(repo RegisterRepository, tokens *TokenService, ttl time.Duration) *RegisterService {
	return &RegisterService{repo: repo, tokens: tokens, ttl: ttl}
}

// Register creates the school and its first admin, then returns a login token
// so the wizard can continue straight into the app.
func (s *RegisterService) Register(ctx context.Context, in RegisterInput) (LoginResult, error) {
	in, err := normalizeRegistration(in)
	if err != nil {
		return LoginResult{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return LoginResult{}, fmt.Errorf("hash password: %w", err)
	}

	created, err := s.repo.RegisterSchoolAdmin(ctx, in, strings.ToLower(in.Email), string(hash))
	if err != nil {
		if database.IsUniqueViolation(err) {
			switch database.ConstraintName(err) {
			case "uq_schools_npsn":
				return LoginResult{}, ErrNPSNTaken
			case "uq_users_username":
				return LoginResult{}, ErrEmailTaken
			}
		}
		return LoginResult{}, fmt.Errorf("register school admin: %w", err)
	}

	token, err := s.tokens.Issue(strconv.FormatInt(created.UserID, 10), []string{"admin"}, created.SchoolID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue access token: %w", err)
	}

	return LoginResult{
		Token:     token,
		ExpiresAt: time.Now().Add(s.ttl).UTC().Format(time.RFC3339),
		UserID:    created.UserID,
		Username:  created.Username,
		Nama:      in.AdminName,
		Roles:     []string{"admin"},
	}, nil
}

func normalizeRegistration(in RegisterInput) (RegisterInput, error) {
	in.SchoolName = strings.TrimSpace(in.SchoolName)
	in.NPSN = strings.TrimSpace(in.NPSN)
	in.Jenjang = strings.ToUpper(strings.TrimSpace(in.Jenjang))
	in.AdminName = strings.TrimSpace(in.AdminName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Phone = digitsOnly(in.Phone)

	if in.SchoolName == "" || len(in.SchoolName) > 120 ||
		in.AdminName == "" || len(in.AdminName) > 100 ||
		in.Jenjang == "" || len(in.Jenjang) > 20 ||
		!npsnPattern.MatchString(in.NPSN) ||
		len(in.Email) > 254 || !emailPattern.MatchString(in.Email) ||
		len(in.Phone) < 9 || len(in.Phone) > 14 ||
		!(strings.HasPrefix(in.Phone, "0") || strings.HasPrefix(in.Phone, "62")) ||
		len(in.Password) < 8 || len(in.Password) > 72 {
		return RegisterInput{}, ErrInvalidRegistration
	}
	return in, nil
}

func digitsOnly(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}
