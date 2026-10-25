package notifications

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("push notification authorization required")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrInvalidInput = errors.New("invalid push subscription")
	ErrNotFound     = errors.New("push subscription not found")
	// ErrDisabled means the server has no complete VAPID configuration;
	// subscription writes and sends answer 503 push_disabled.
	ErrDisabled = errors.New("push notifications are disabled")
)

// Config is the VAPID setup read from the IFTER_WEBPUSH_* environment.
type Config struct {
	Enabled    bool
	PublicKey  string
	PrivateKey string
	Subject    string
}

// ConfigView is GET /push/config; the private key never leaves the server.
type ConfigView struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
}

type Actor struct {
	ID    string
	Roles []string
}

// Payload is the JSON body delivered to the service worker.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
}

type SubscriptionInput struct {
	Endpoint  string
	P256DH    string
	Auth      string
	UserAgent string
}

// Subscription is the external shape: keys are write-only.
type Subscription struct {
	ID        string    `json:"id"`
	Endpoint  string    `json:"endpoint"`
	UserAgent *string   `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SendInput struct {
	Title string
	Body  string
}

type SendResult struct {
	Sent   int `json:"sent"`
	Failed int `json:"failed"`
}

// SubscriptionKeys are the encrypted-push credentials handed to the sender.
type SubscriptionKeys struct {
	Endpoint string
	P256DH   string
	Auth     string
}

// Sender delivers one encrypted payload. The response status is returned even
// alongside an error so expired subscriptions (404/410) can be pruned.
type Sender interface {
	Send(ctx context.Context, keys SubscriptionKeys, payload []byte) (int, error)
}
