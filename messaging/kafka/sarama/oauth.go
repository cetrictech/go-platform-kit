package sarama

import (
	"context"
	s "github.com/IBM/sarama"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"time"
)

type oauthProvider struct {
	provider kafka.TokenProvider
	timeout  time.Duration
}

func (p oauthProvider) Token() (*s.AccessToken, error) {
	ctx, cancel := kafka.AuthenticationContext(context.Background(), p.timeout)
	defer cancel()
	v, e := p.provider(ctx)
	if e != nil {
		return nil, e
	}
	if e = kafka.ValidateOAuth(v); e != nil {
		return nil, e
	}
	return &s.AccessToken{Token: v.Token, Extensions: v.Extensions}, nil
}
