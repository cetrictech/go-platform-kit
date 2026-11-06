package kafka

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/encodingx"
)

// PublishValue serializes a typed payload while retaining topic/key/headers metadata. Nil serializer uses bounded JSON.
func PublishValue(ctx context.Context, p Producer, serializer encodingx.Serializer, metadata Message, value any) error {
	if p == nil {
		return errors.New("kafka: producer required")
	}
	if serializer == nil {
		serializer = encodingx.JSON{}
	}
	b, e := serializer.Encode(value)
	if e != nil {
		return e
	}
	metadata.Value = b
	return p.Publish(ctx, metadata)
}

// DecodeHandler decodes before invoking the handler; decode failures are returned so the consumer cannot commit the record.
func DecodeHandler[T any](serializer encodingx.Serializer, handler func(context.Context, Message, T) error) Handler {
	if serializer == nil {
		serializer = encodingx.JSON{}
	}
	return func(ctx context.Context, m Message) error {
		if handler == nil {
			return errors.New("kafka: decoded handler required")
		}
		var value T
		if e := serializer.Decode(m.Value, &value); e != nil {
			return e
		}
		return handler(ctx, m, value)
	}
}
