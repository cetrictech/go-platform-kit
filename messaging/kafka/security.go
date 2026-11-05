package kafka

import (
	"context"
	"errors"
	"time"
)

// Credentials are fetched at authentication time. Providers must honor cancellation and avoid returning secrets in errors.
type Credentials struct{ Username, Password string }
type CredentialProvider func(context.Context) (Credentials, error)

// OAuthToken is an opaque SASL bearer token. The issuing authority determines its authorization scope.
type OAuthToken struct {
	Token      string
	Extensions map[string]string
	ExpiresAt  time.Time
}
type TokenProvider func(context.Context) (OAuthToken, error)

// AuthenticationContext gives credential providers a finite deadline without retaining a request context in a client.
func AuthenticationContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// ValidateOAuth rejects absent or already expired tokens before sending them to a broker.
func ValidateOAuth(t OAuthToken) error {
	if t.Token == "" || !t.ExpiresAt.IsZero() && !time.Now().Before(t.ExpiresAt) {
		return errors.New("kafka: provider returned an absent or expired OAuth token")
	}
	return nil
}
