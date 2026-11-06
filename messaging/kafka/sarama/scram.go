package sarama

import (
	"context"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"github.com/xdg-go/scram"
	"time"
)

type scramClient struct {
	provider     kafka.CredentialProvider
	timeout      time.Duration
	hash         scram.HashGeneratorFcn
	conversation *scram.ClientConversation
}

// Begin atomically attempts to acquire a fingerprint-bound processing lease.
func (c *scramClient) Begin(user, password, authzID string) error {
	if c.provider != nil {
		ctx, cancel := kafka.AuthenticationContext(context.Background(), c.timeout)
		v, e := c.provider(ctx)
		cancel()
		if e != nil {
			return e
		}
		user, password = v.Username, v.Password
	}
	client, e := c.hash.NewClient(user, password, authzID)
	if e != nil {
		return e
	}
	c.conversation = client.NewConversation()
	return nil
}
func (c *scramClient) Step(challenge string) (string, error) { return c.conversation.Step(challenge) }
func (c *scramClient) Done() bool                            { return c.conversation.Done() }
