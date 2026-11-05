package sarama

import (
	"github.com/xdg-go/scram"
)

type scramClient struct {
	hash         scram.HashGeneratorFcn
	conversation *scram.ClientConversation
}

// Begin atomically attempts to acquire a fingerprint-bound processing lease.
func (c *scramClient) Begin(user, password, authzID string) error {
	client, e := c.hash.NewClient(user, password, authzID)
	if e != nil {
		return e
	}
	c.conversation = client.NewConversation()
	return nil
}
func (c *scramClient) Step(challenge string) (string, error) { return c.conversation.Step(challenge) }
func (c *scramClient) Done() bool                            { return c.conversation.Done() }
