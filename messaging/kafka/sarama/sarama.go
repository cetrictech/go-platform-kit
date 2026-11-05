// Package sarama implements Kafka contracts using IBM Sarama.
package sarama

import (
	"context"
	"crypto/tls"
	"errors"
	s "github.com/IBM/sarama"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"github.com/xdg-go/scram"
	"sync"
	"sync/atomic"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Workers                        int
	ManualPartition                bool
	SASL, Username, Password       string
	Linger                         time.Duration
	BatchMessages, MaxMessageBytes int
	OffsetReset                    string
	Compression                    string
	DisableIdempotence             bool
	Acknowledgments                string
	RetryMax                       int
	RetryBackoff                   time.Duration
	AllowPlaintext                 bool
	Brokers, Topics                []string
	Group, ClientID                string
	TLS                            *tls.Config
	NativeConfig                   *s.Config
}

func config(c Config) (*s.Config, error) {
	if len(c.Brokers) == 0 {
		return nil, errors.New("sarama: brokers required")
	}
	cfg := s.NewConfig()
	cfg.ClientID = c.ClientID
	if cfg.ClientID == "" {
		cfg.ClientID = "go-platform-kit"
	}
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.RequiredAcks = s.WaitForAll
	cfg.Producer.Idempotent = true
	cfg.Net.MaxOpenRequests = 1
	cfg.Consumer.Offsets.Initial = s.OffsetOldest
	cfg.Consumer.Offsets.AutoCommit.Enable = false
	cfg.Consumer.Return.Errors = true
	switch c.OffsetReset {
	case "", "earliest":
	case "latest":
		cfg.Consumer.Offsets.Initial = s.OffsetNewest
	default:
		return nil, errors.New("sarama: invalid offset reset")
	}
	if c.RetryMax < 0 || c.RetryBackoff < 0 || c.Linger < 0 || c.BatchMessages < 0 || c.MaxMessageBytes < 0 {
		return nil, errors.New("sarama: invalid producer tuning")
	}
	if c.ManualPartition {
		cfg.Producer.Partitioner = s.NewManualPartitioner
	}
	if c.DisableIdempotence {
		cfg.Producer.Idempotent = false
	}
	switch c.Acknowledgments {
	case "", "all":
	case "leader":
		if !c.DisableIdempotence {
			return nil, errors.New("sarama: leader acknowledgments require disabled idempotence")
		}
		cfg.Producer.RequiredAcks = s.WaitForLocal
	default:
		return nil, errors.New("sarama: invalid acknowledgments")
	}
	if c.Linger > 0 {
		cfg.Producer.Flush.Frequency = c.Linger
	}
	if c.BatchMessages > 0 {
		cfg.Producer.Flush.Messages = c.BatchMessages
	}
	if c.MaxMessageBytes > 0 {
		cfg.Producer.MaxMessageBytes = c.MaxMessageBytes
	}
	if c.RetryMax > 0 {
		cfg.Producer.Retry.Max = c.RetryMax
	}
	if c.RetryBackoff > 0 {
		cfg.Producer.Retry.Backoff = c.RetryBackoff
	}
	switch c.Compression {
	case "", "none":
	case "gzip":
		cfg.Producer.Compression = s.CompressionGZIP
	case "snappy":
		cfg.Producer.Compression = s.CompressionSnappy
	case "lz4":
		cfg.Producer.Compression = s.CompressionLZ4
	case "zstd":
		cfg.Producer.Compression = s.CompressionZSTD
	default:
		return nil, errors.New("sarama: invalid compression")
	}
	if c.TLS != nil {
		if c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
			return nil, errors.New("sarama: TLS verification required")
		}
		cfg.Net.TLS.Enable = true
		cfg.Net.TLS.Config = c.TLS.Clone()
	}
	if c.SASL != "" {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = c.Username
		cfg.Net.SASL.Password = c.Password
		switch c.SASL {
		case "PLAIN":
			cfg.Net.SASL.Mechanism = s.SASLTypePlaintext
		case "SCRAM-SHA-256":
			cfg.Net.SASL.Mechanism = s.SASLTypeSCRAMSHA256
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() s.SCRAMClient { return &scramClient{hash: scram.SHA256} }
		case "SCRAM-SHA-512":
			cfg.Net.SASL.Mechanism = s.SASLTypeSCRAMSHA512
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() s.SCRAMClient { return &scramClient{hash: scram.SHA512} }
		default:
			return nil, errors.New("sarama: unsupported SASL mechanism")
		}
	}
	if c.NativeConfig != nil {
		copy := *c.NativeConfig
		cfg = &copy
		cfg.Producer.Return.Successes = true
		cfg.Producer.Return.Errors = true
		cfg.Consumer.Return.Errors = true
		cfg.Consumer.Offsets.AutoCommit.Enable = false
	}
	if cfg.Net.TLS.Config != nil && cfg.Net.TLS.Config.InsecureSkipVerify && cfg.Net.TLS.Config.VerifyPeerCertificate == nil && cfg.Net.TLS.Config.VerifyConnection == nil {
		return nil, errors.New("sarama: TLS verification required")
	}
	if cfg.Net.SASL.Enable && !cfg.Net.TLS.Enable {
		return nil, errors.New("sarama: TLS required for SASL")
	}
	if !cfg.Net.TLS.Enable && !c.AllowPlaintext {
		return nil, errors.New("sarama: TLS required; explicitly opt into plaintext for local development")
	}
	return cfg, cfg.Validate()
}

type delivery struct {
	result   chan error
	callback func(kafka.Delivery, error)
}

// Producer owns an acknowledged Kafka producer.
type Producer struct {
	producer s.AsyncProducer
	mu       sync.RWMutex
	closed   bool
	pending  atomic.Int64
	done     chan struct{}
}

// NewProducer constructs an owned Kafka producer with validated configuration.
func NewProducer(c Config) (*Producer, error) {
	cfg, e := config(c)
	if e != nil {
		return nil, e
	}
	p, e := s.NewAsyncProducer(c.Brokers, cfg)
	if e != nil {
		return nil, e
	}
	out := &Producer{producer: p, done: make(chan struct{})}
	go out.dispatch()
	return out, nil
}
func (p *Producer) dispatch() {
	defer close(p.done)
	successes, failures := p.producer.Successes(), p.producer.Errors()
	for successes != nil || failures != nil {
		select {
		case m, ok := <-successes:
			if !ok {
				successes = nil
				continue
			}
			if d, ok := m.Metadata.(*delivery); ok {
				p.pending.Add(-1)
				d.complete(m, nil)
			}
		case e, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			if d, ok := e.Msg.Metadata.(*delivery); ok {
				p.pending.Add(-1)
				d.complete(e.Msg, e.Err)
			}
		}
	}
}

// Publish waits for broker acknowledgment. Cancellation after enqueue has unknown delivery outcome.
func (p *Producer) Publish(ctx context.Context, m kafka.Message) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	d := &delivery{result: make(chan error, 1)}
	msg := &s.ProducerMessage{Topic: m.Topic, Key: s.ByteEncoder(m.Key), Value: s.ByteEncoder(m.Value), Timestamp: m.Timestamp, Partition: m.Partition, Metadata: d}
	for k, v := range m.Headers {
		msg.Headers = append(msg.Headers, s.RecordHeader{Key: []byte(k), Value: v})
	}
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return errors.New("sarama: producer closed")
	}
	p.pending.Add(1)
	select {
	case p.producer.Input() <- msg:
		p.mu.RUnlock()
	case <-ctx.Done():
		p.pending.Add(-1)
		p.mu.RUnlock()
		return ctx.Err()
	}
	select {
	case e := <-d.result:
		return e
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishBatch publishes every supplied message, returning a delivery error if any operation fails.
func (p *Producer) PublishBatch(ctx context.Context, msgs []kafka.Message) error {
	for _, m := range msgs {
		if e := p.Publish(ctx, m); e != nil {
			return e
		}
	}
	return nil
}

// Flush waits for pending publication outcomes, bounded by context cancellation.
func (p *Producer) Flush(ctx context.Context) error {
	timer := time.NewTicker(time.Millisecond)
	defer timer.Stop()
	for p.pending.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (p *Producer) Close() error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.producer.AsyncClose()
	}
	p.mu.Unlock()
	<-p.done
	return nil
}

// Consumer owns a Kafka consumer group client.
type Consumer struct {
	workers int
	group   s.ConsumerGroup
	topics  []string
	running atomic.Bool
}

// NewConsumer constructs an owned consumer group client.
func NewConsumer(c Config) (*Consumer, error) {
	if c.Workers < 0 || c.Workers > 256 {
		return nil, errors.New("sarama: workers must be 0..256")
	}
	if c.Workers == 0 {
		c.Workers = 8
	}
	if c.Group == "" || len(c.Topics) == 0 {
		return nil, errors.New("sarama: group and topics required")
	}
	cfg, e := config(c)
	if e != nil {
		return nil, e
	}
	g, e := s.NewConsumerGroup(c.Brokers, c.Group, cfg)
	if e != nil {
		return nil, e
	}
	return &Consumer{group: g, topics: append([]string(nil), c.Topics...), workers: c.Workers}, nil
}

type handler struct {
	sem    chan struct{}
	h      kafka.Handler
	err    chan error
	cancel context.CancelFunc
}

func (*handler) Setup(s.ConsumerGroupSession) error   { return nil }
func (*handler) Cleanup(s.ConsumerGroupSession) error { return nil }
func (h *handler) ConsumeClaim(session s.ConsumerGroupSession, claim s.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return session.Context().Err()
		case m, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			headers := map[string][]byte{}
			for _, v := range m.Headers {
				headers[string(v.Key)] = append([]byte(nil), v.Value...)
			}
			msg := kafka.Message{Topic: m.Topic, Key: append([]byte(nil), m.Key...), Value: append([]byte(nil), m.Value...), Headers: headers, Timestamp: m.Timestamp, Partition: m.Partition, Offset: m.Offset}
			if e := h.invoke(session.Context(), msg); e != nil {
				select {
				case h.err <- e:
				default:
				}
				h.cancel()
				return e
			}
			session.MarkMessage(m, "")
			session.Commit()
		}
	}
}

// Consume commits after successful handling. Sarama reports commit errors through its group error channel.
func (c *Consumer) Consume(ctx context.Context, h kafka.Handler) error {
	if h == nil || !c.running.CompareAndSwap(false, true) {
		return errors.New("sarama: invalid handler or already consuming")
	}
	defer c.running.Store(false)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-c.group.Errors():
				if !ok {
					return
				}
				select {
				case errs <- e:
				default:
				}
				cancel()
				return
			}
		}
	}()
	defer func() { cancel(); <-done }()
	sem := make(chan struct{}, c.workers)
	for {
		e := c.group.Consume(ctx, c.topics, &handler{h: h, err: errs, cancel: cancel, sem: sem})
		select {
		case failure := <-errs:
			return failure
		default:
		}
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// Pause pauses fetching for the specified scope.
func (c *Consumer) Pause() { c.group.PauseAll() }

// Resume resumes fetching for the specified scope.
func (c *Consumer) Resume() { c.group.ResumeAll() }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Consumer) Close() error { return c.group.Close() }

// Admin owns a Kafka administration client.
type Admin struct{ admin s.ClusterAdmin }

// NewAdmin constructs an owned Kafka administration client.
func NewAdmin(c Config) (*Admin, error) {
	cfg, e := config(c)
	if e != nil {
		return nil, e
	}
	a, e := s.NewClusterAdmin(c.Brokers, cfg)
	if e != nil {
		return nil, e
	}
	return &Admin{a}, nil
}

// CreateTopic creates a broker topic with explicit partition and replication settings.
func (a *Admin) CreateTopic(ctx context.Context, t kafka.Topic) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return a.admin.CreateTopic(t.Name, &s.TopicDetail{NumPartitions: t.Partitions, ReplicationFactor: t.ReplicationFactor, ConfigEntries: t.Config}, false)
}

// DeleteTopic deletes the named broker topic.
func (a *Admin) DeleteTopic(ctx context.Context, name string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return a.admin.DeleteTopic(name)
}

// ListTopics returns available topic names and partition IDs.
func (a *Admin) ListTopics(ctx context.Context) ([]kafka.TopicInfo, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	m, e := a.admin.ListTopics()
	if e != nil {
		return nil, e
	}
	var out []kafka.TopicInfo
	for name, t := range m {
		v := kafka.TopicInfo{Name: name}
		for p := range t.ReplicaAssignment {
			v.Partitions = append(v.Partitions, p)
		}
		out = append(out, v)
	}
	return out, nil
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (a *Admin) Close() error { return a.admin.Close() }

var _ kafka.Producer = (*Producer)(nil)
var _ kafka.Consumer = (*Consumer)(nil)
var _ kafka.Admin = (*Admin)(nil)

func (d *delivery) complete(m *s.ProducerMessage, e error) {
	if d.result != nil {
		d.result <- e
	}
	if d.callback != nil {
		d.callback(kafka.Delivery{Topic: m.Topic, Partition: m.Partition, Offset: m.Offset, Timestamp: m.Timestamp}, e)
	}
}

// PublishAsync queues a message; callback receives the broker delivery outcome. Cancellation after queueing does not retract it.
// Callbacks run on the delivery dispatcher and must return promptly without calling Close.
func (p *Producer) PublishAsync(ctx context.Context, m kafka.Message, callback func(kafka.Delivery, error)) error {
	if callback == nil {
		return errors.New("sarama: delivery callback required")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	d := &delivery{callback: callback}
	msg := &s.ProducerMessage{Topic: m.Topic, Key: s.ByteEncoder(m.Key), Value: s.ByteEncoder(m.Value), Timestamp: m.Timestamp, Partition: m.Partition, Metadata: d}
	for k, v := range m.Headers {
		msg.Headers = append(msg.Headers, s.RecordHeader{Key: []byte(k), Value: v})
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return errors.New("sarama: producer closed")
	}
	p.pending.Add(1)
	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		p.pending.Add(-1)
		return ctx.Err()
	}
}

// ConsumeAcknowledged commits only when the handler explicitly calls Ack and then returns nil.
func (c *Consumer) ConsumeAcknowledged(ctx context.Context, h kafka.AckHandler) error {
	return c.Consume(ctx, kafka.RequireAck(h))
}

var _ kafka.ManualConsumer = (*Consumer)(nil)

// PublishWithDelivery waits for the configured broker acknowledgment and returns assigned metadata.
func (p *Producer) PublishWithDelivery(ctx context.Context, m kafka.Message) (kafka.Delivery, error) {
	type result struct {
		d kafka.Delivery
		e error
	}
	ch := make(chan result, 1)
	if e := p.PublishAsync(ctx, m, func(d kafka.Delivery, e error) { ch <- result{d, e} }); e != nil {
		return kafka.Delivery{}, e
	}
	select {
	case r := <-ch:
		return r.d, r.e
	case <-ctx.Done():
		return kafka.Delivery{}, ctx.Err()
	}
}

func (h *handler) invoke(ctx context.Context, m kafka.Message) error {
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
		return kafka.Recover(h.h)(ctx, m)
	case <-ctx.Done():
		return ctx.Err()
	}
}
