// Package kafka defines driver-independent Kafka contracts and failure handling.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/retry"
	"strconv"
	"sync/atomic"
	"time"
)

// Message contains transport-independent message data.
type Message struct {
	Topic      string
	Key, Value []byte
	Headers    map[string][]byte
	Timestamp  time.Time
	Partition  int32
	Offset     int64
}

// Producer owns an acknowledged Kafka producer.
type Producer interface {
	Publish(context.Context, Message) error
	PublishBatch(context.Context, []Message) error
	Flush(context.Context) error
	Close() error
}

// Handler processes one message with cancellation support.
type Handler func(context.Context, Message) error

// Consumer owns a Kafka consumer group client.
type Consumer interface {
	Consume(context.Context, Handler) error
	Close() error
}

// Delivery contains broker-assigned delivery metadata.
type Delivery struct {
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Topic describes explicit topic creation settings.
type Topic struct {
	Name              string
	Partitions        int32
	ReplicationFactor int16
	Config            map[string]*string
}

// TopicInfo contains basic topic metadata.
type TopicInfo struct {
	Name       string
	Partitions []int32
	Internal   bool
}

// Admin owns a Kafka administration client.
type Admin interface {
	CreateTopic(context.Context, Topic) error
	DeleteTopic(context.Context, string) error
	ListTopics(context.Context) ([]TopicInfo, error)
	Close() error
}

// Recover reports panics as errors so the consumer does not commit failed messages.
func Recover(h Handler) Handler {
	return func(ctx context.Context, m Message) (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("kafka handler panic: %v", p)
			}
		}()
		return h(ctx, m)
	}
}

// WithFailure retries an explicitly idempotent handler, then publishes a DLQ message.
// A nil return after DLQ means the consumer may commit the source offset. DLQ failure leaves it uncommitted.
// Publish and source commit are separate operations; consumers must deduplicate DLQ deliveries.
func WithFailure(h Handler, p retry.Policy, producer Producer, dlq string) Handler {
	return func(ctx context.Context, m Message) error {
		attempts := 0
		e := retry.Do(ctx, p, func(ctx context.Context) error { attempts++; return Recover(h)(ctx, m) })
		if e == nil {
			return nil
		}
		if producer == nil || dlq == "" {
			return e
		}
		headers := make(map[string][]byte, len(m.Headers)+5)
		for k, v := range m.Headers {
			headers[k] = append([]byte(nil), v...)
		}
		headers["original-topic"] = []byte(m.Topic)
		headers["original-partition"] = []byte(strconv.Itoa(int(m.Partition)))
		headers["original-offset"] = []byte(strconv.FormatInt(m.Offset, 10))
		headers["attempts"] = []byte(strconv.Itoa(attempts))
		headers["failed-at"] = []byte(time.Now().UTC().Format(time.RFC3339Nano))
		out := m
		out.Topic = dlq
		out.Headers = headers
		out.Partition = -1
		if err := producer.Publish(ctx, out); err != nil {
			return errors.Join(e, err)
		}
		return nil
	}
}

var ErrUnacknowledged = errors.New("kafka: handler returned without acknowledgment")

// AcknowledgedMessage requires Ack before the handler returns successfully. Ack requests the post-handler offset commit; it is not a broker commit receipt.
type AcknowledgedMessage struct {
	Message
	Ack func(context.Context) error
}

// AckHandler handles a message requiring explicit acknowledgment intent.
type AckHandler func(context.Context, AcknowledgedMessage) error

// ManualConsumer extends consumption with required explicit acknowledgment before successful return.
type ManualConsumer interface {
	Consumer
	ConsumeAcknowledged(context.Context, AckHandler) error
}

// RequireAck prevents accidental source commits when a handler forgets to acknowledge.
func RequireAck(h AckHandler) Handler {
	return func(ctx context.Context, m Message) error {
		if h == nil {
			return errors.New("kafka: nil acknowledgment handler")
		}
		var acknowledged atomic.Bool
		ack := func(ctx context.Context) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			acknowledged.Store(true)
			return nil
		}
		if e := h(ctx, AcknowledgedMessage{m, ack}); e != nil {
			return e
		}
		if !acknowledged.Load() {
			return ErrUnacknowledged
		}
		return nil
	}
}

// PartitionInfo describes broker placement at one point in time.
type PartitionInfo struct {
	ID, Leader               int32
	Replicas, InSyncReplicas []int32
}

// TopicDescription contains topic metadata and broker partition placement.
type TopicDescription struct {
	Name       string
	Internal   bool
	Partitions []PartitionInfo
}

// GroupMember identifies a current consumer group member.
type GroupMember struct{ ID, ClientID, Host string }

// GroupInfo describes consumer group state and membership.
type GroupInfo struct {
	Name, State, ProtocolType string
	Members                   []GroupMember
}

// Offset is the next offset a consumer group will resume from; -1 indicates no committed offset.
type Offset struct {
	Topic     string
	Partition int32
	Offset    int64
	Metadata  string
}

// Inspector extends basic administration with topic, group and committed-offset inspection.
type Inspector interface {
	Admin
	DescribeTopic(context.Context, string) (TopicDescription, error)
	ListGroups(context.Context) ([]GroupInfo, error)
	DescribeGroup(context.Context, string) (GroupInfo, error)
	Offsets(context.Context, string) ([]Offset, error)
}
