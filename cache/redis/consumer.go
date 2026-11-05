package redis

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StreamConsumerConfig names an existing consumer group. Pending-entry reclamation is explicitly managed with XAutoClaim.
type StreamConsumerConfig struct {
	Stream, Group, Consumer string
	Count                   int64
	Block                   time.Duration
}

// ConsumeGroup reads new group messages serially and acknowledges only after successful processing.
// A handler failure/panic leaves the entry pending; callers choose retry or reclaim policy explicitly.
func (c *Client) ConsumeGroup(ctx context.Context, cfg StreamConsumerConfig, handler func(context.Context, StreamMessage) error) error {
	if cfg.Stream == "" || cfg.Group == "" || cfg.Consumer == "" || handler == nil || cfg.Count < 0 || cfg.Count > 1000 || cfg.Block < 0 {
		return errors.New("redis: invalid stream consumer")
	}
	if cfg.Count == 0 {
		cfg.Count = 100
	}
	if cfg.Block == 0 {
		cfg.Block = time.Second
	}
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		streams, e := c.XReadGroup(ctx, cfg.Stream, cfg.Group, cfg.Consumer, ">", cfg.Count, cfg.Block)
		if errors.Is(e, ErrNotFound) {
			continue
		}
		if e != nil {
			return e
		}
		for _, s := range streams {
			for _, m := range s.Messages {
				if e = streamHandle(ctx, m, handler); e != nil {
					return e
				}
				if _, e = c.XAck(ctx, cfg.Stream, cfg.Group, m.ID); e != nil {
					return e
				}
			}
		}
	}
}
func streamHandle(ctx context.Context, m StreamMessage, h func(context.Context, StreamMessage) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("redis stream handler panic: %v", p)
		}
	}()
	return h(ctx, m)
}
