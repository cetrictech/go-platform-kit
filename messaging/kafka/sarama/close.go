package sarama

import "context"

// CloseContext cancels and drains handlers before closing the group. On deadline, native cleanup continues and may be awaited again.
func (c *Consumer) CloseContext(ctx context.Context) error {
	return c.life.Shutdown(ctx, c.group.Close)
}
