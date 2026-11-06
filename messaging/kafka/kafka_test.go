package kafka

import (
	"context"
	"testing"
)

func TestRecovery(t *testing.T) {
	e := Recover(func(context.Context, Message) error { panic("bad") })(context.Background(), Message{})
	if e == nil {
		t.Fatal("lost panic")
	}
}

func TestAcknowledgment(t *testing.T) {
	ctx := context.Background()
	if e := RequireAck(func(context.Context, AcknowledgedMessage) error { return nil })(ctx, Message{}); e != ErrUnacknowledged {
		t.Fatal(e)
	}
	if e := RequireAck(func(ctx context.Context, m AcknowledgedMessage) error { return m.Ack(ctx) })(ctx, Message{}); e != nil {
		t.Fatal(e)
	}
}

func BenchmarkConsumerProcessing(b *testing.B) {
	ctx := context.Background()
	count := 0
	h := Recover(func(_ context.Context, m Message) error { count += len(m.Value); return nil })
	m := Message{Value: make([]byte, 256)}
	for b.Loop() {
		if e := h(ctx, m); e != nil {
			b.Fatal(e)
		}
	}
	if count == 0 {
		b.Fatal("handler was not invoked")
	}
}
