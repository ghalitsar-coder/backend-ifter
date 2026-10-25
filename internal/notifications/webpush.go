package notifications

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// webPushSender delivers encrypted payloads with the VAPID credentials from
// Config. The client timeout bounds a single failing push service.
type webPushSender struct {
	cfg    Config
	client *http.Client
}

func newWebPushSender(cfg Config) *webPushSender {
	return &webPushSender{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *webPushSender) Send(ctx context.Context, keys SubscriptionKeys, payload []byte) (int, error) {
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: keys.Endpoint,
		Keys: webpush.Keys{
			P256dh: keys.P256DH,
			Auth:   keys.Auth,
		},
	}, &webpush.Options{
		Subscriber:      s.cfg.Subject,
		VAPIDPublicKey:  s.cfg.PublicKey,
		VAPIDPrivateKey: s.cfg.PrivateKey,
		TTL:             60,
		HTTPClient:      s.client,
	})
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, fmt.Errorf("push service answered %d", response.StatusCode)
	}
	return response.StatusCode, nil
}
