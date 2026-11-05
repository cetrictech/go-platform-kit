// Package franz implements Kafka using franz-go. Publish waits for broker acknowledgment.
package franz

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/internal/consumerlife"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/oauth"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"net"
	"sync"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	// DialContext supplies network connections, including advertised broker addresses.
	DialContext                               func(context.Context, string, string) (net.Conn, error)
	Credentials                               kafka.CredentialProvider
	OAuthToken                                kafka.TokenProvider
	CredentialTimeout                         time.Duration
	AutoCommit                                bool
	CommitInterval                            time.Duration
	Workers                                   int
	ManualPartition                           bool
	OffsetReset                               string
	Compression                               string
	DisableIdempotence                        bool
	Acknowledgments                           string
	RetryMax                                  int
	RetryBackoff                              time.Duration
	AllowPlaintext                            bool
	Brokers, Topics                           []string
	Group, ClientID, Username, Password, SASL string
	TLS                                       *tls.Config
	Linger                                    time.Duration
	BatchBytes, MaxBufferedRecords            int
	NativeOptions                             []kgo.Opt
}

func options(c Config) ([]kgo.Opt, error) {
	if len(c.Brokers) == 0 {
		return nil, errors.New("franz: brokers required")
	}
	if c.TLS == nil && !c.AllowPlaintext {
		return nil, errors.New("franz: TLS required; explicitly opt into plaintext for local development")
	}
	if c.OffsetReset != "" && c.OffsetReset != "earliest" && c.OffsetReset != "latest" {
		return nil, errors.New("franz: invalid offset reset")
	}
	if c.RetryMax < 0 || c.RetryBackoff < 0 || c.CredentialTimeout < 0 || c.CommitInterval < 0 {
		return nil, errors.New("franz: invalid retry config")
	}
	opts := []kgo.Opt{kgo.SeedBrokers(c.Brokers...), kgo.ClientID(c.ClientID)}
	if c.DialContext != nil && c.TLS == nil {
		opts = append(opts, kgo.Dialer(c.DialContext))
	}
	if c.OffsetReset == "latest" {
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
	} else {
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	}
	if c.ManualPartition {
		opts = append(opts, kgo.RecordPartitioner(kgo.ManualPartitioner()))
	}
	if c.DisableIdempotence {
		opts = append(opts, kgo.DisableIdempotentWrite())
	}
	switch c.Acknowledgments {
	case "", "all":
	case "leader":
		if !c.DisableIdempotence {
			return nil, errors.New("franz: leader acknowledgments require disabled idempotence")
		}
		opts = append(opts, kgo.RequiredAcks(kgo.LeaderAck()))
	default:
		return nil, errors.New("franz: invalid acknowledgments")
	}
	if c.Compression != "" {
		var codec kgo.CompressionCodec
		switch c.Compression {
		case "none":
			codec = kgo.NoCompression()
		case "gzip":
			codec = kgo.GzipCompression()
		case "snappy":
			codec = kgo.SnappyCompression()
		case "lz4":
			codec = kgo.Lz4Compression()
		case "zstd":
			codec = kgo.ZstdCompression()
		default:
			return nil, errors.New("franz: invalid compression")
		}
		opts = append(opts, kgo.ProducerBatchCompression(codec))
	}
	if c.RetryMax > 0 {
		opts = append(opts, kgo.RecordRetries(c.RetryMax))
	}
	if c.RetryBackoff > 0 {
		opts = append(opts, kgo.RetryBackoffFn(func(int) time.Duration { return c.RetryBackoff }))
	}
	if c.TLS != nil {
		if c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
			return nil, errors.New("franz: TLS verification required")
		}
		if c.DialContext == nil {
			opts = append(opts, kgo.DialTLSConfig(c.TLS.Clone()))
		} else {
			tlsConfig := c.TLS.Clone()
			dial := c.DialContext
			opts = append(opts, kgo.Dialer(func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, err := dial(ctx, network, address)
				if err != nil {
					return nil, err
				}
				cfg := tlsConfig.Clone()
				if cfg.ServerName == "" {
					cfg.ServerName, _, err = net.SplitHostPort(address)
					if err != nil {
						conn.Close()
						return nil, err
					}
				}
				secured := tls.Client(conn, cfg)
				if err := secured.HandshakeContext(ctx); err != nil {
					conn.Close()
					return nil, err
				}
				return secured, nil
			}))
		}
	}
	if c.SASL != "" {
		if c.TLS == nil {
			return nil, errors.New("franz: TLS required for SASL")
		}
		credentials := func(ctx context.Context) (kafka.Credentials, error) {
			ctx, cancel := kafka.AuthenticationContext(ctx, c.CredentialTimeout)
			defer cancel()
			if c.Credentials != nil {
				return c.Credentials(ctx)
			}
			return kafka.Credentials{Username: c.Username, Password: c.Password}, nil
		}
		switch c.SASL {
		case "PLAIN":
			opts = append(opts, kgo.SASL(plain.Plain(func(ctx context.Context) (plain.Auth, error) {
				v, e := credentials(ctx)
				return plain.Auth{User: v.Username, Pass: v.Password}, e
			})))
		case "SCRAM-SHA-256":
			opts = append(opts, kgo.SASL(scram.Sha256(func(ctx context.Context) (scram.Auth, error) {
				v, e := credentials(ctx)
				return scram.Auth{User: v.Username, Pass: v.Password}, e
			})))
		case "SCRAM-SHA-512":
			opts = append(opts, kgo.SASL(scram.Sha512(func(ctx context.Context) (scram.Auth, error) {
				v, e := credentials(ctx)
				return scram.Auth{User: v.Username, Pass: v.Password}, e
			})))
		case "OAUTHBEARER":
			if c.OAuthToken == nil {
				return nil, errors.New("franz: OAuth token provider required")
			}
			opts = append(opts, kgo.SASL(oauth.Oauth(func(ctx context.Context) (oauth.Auth, error) {
				ctx, cancel := kafka.AuthenticationContext(ctx, c.CredentialTimeout)
				defer cancel()
				v, e := c.OAuthToken(ctx)
				if e != nil {
					return oauth.Auth{}, e
				}
				if e = kafka.ValidateOAuth(v); e != nil {
					return oauth.Auth{}, e
				}
				return oauth.Auth{Token: v.Token, Extensions: v.Extensions}, nil
			})))
		default:
			return nil, errors.New("franz: unsupported SASL mechanism")
		}
	}
	return opts, nil
}

// Producer owns an acknowledged Kafka producer.
type Producer struct{ client *kgo.Client }

// NewProducer constructs an owned Kafka producer with validated configuration.
func NewProducer(c Config) (*Producer, error) {
	if c.Linger < 0 || c.BatchBytes < 0 || c.MaxBufferedRecords < 0 {
		return nil, errors.New("franz: invalid producer tuning")
	}
	o, e := options(c)
	if e != nil {
		return nil, e
	}
	if c.Linger > 0 {
		o = append(o, kgo.ProducerLinger(c.Linger))
	}
	if c.BatchBytes > 0 {
		o = append(o, kgo.ProducerBatchMaxBytes(int32(c.BatchBytes)))
	}
	if c.MaxBufferedRecords > 0 {
		o = append(o, kgo.MaxBufferedRecords(c.MaxBufferedRecords))
	}
	o = append(o, c.NativeOptions...)
	cl, e := kgo.NewClient(o...)
	if e != nil {
		return nil, e
	}
	return &Producer{cl}, nil
}
func record(m kafka.Message) *kgo.Record {
	r := &kgo.Record{Topic: m.Topic, Key: append([]byte(nil), m.Key...), Value: append([]byte(nil), m.Value...), Timestamp: m.Timestamp, Partition: m.Partition}
	for k, v := range m.Headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: k, Value: append([]byte(nil), v...)})
	}
	return r
}
func message(r *kgo.Record) kafka.Message {
	h := make(map[string][]byte, len(r.Headers))
	for _, v := range r.Headers {
		h[v.Key] = append([]byte(nil), v.Value...)
	}
	return kafka.Message{Topic: r.Topic, Key: append([]byte(nil), r.Key...), Value: append([]byte(nil), r.Value...), Headers: h, Timestamp: r.Timestamp, Partition: r.Partition, Offset: r.Offset}
}

// Publish publishes with the configured acknowledgment policy.
func (p *Producer) Publish(ctx context.Context, m kafka.Message) error {
	_, err := p.PublishWithDelivery(ctx, m)
	return err
}

// PublishBatch publishes every supplied message, returning a delivery error if any operation fails.
func (p *Producer) PublishBatch(ctx context.Context, msgs []kafka.Message) error {
	results := make(chan error, len(msgs))
	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.client.Produce(ctx, record(m), func(_ *kgo.Record, err error) { results <- err })
	}
	var failures []error
	for range msgs {
		select {
		case err := <-results:
			if err != nil {
				failures = append(failures, err)
			}
		case <-ctx.Done():
			return errors.Join(append(failures, ctx.Err())...)
		}
	}
	return errors.Join(failures...)
}

// PublishAsync queues publication and delivers its broker outcome through a callback that must return promptly.
func (p *Producer) PublishAsync(ctx context.Context, m kafka.Message, callback func(kafka.Delivery, error)) {
	p.client.Produce(ctx, record(m), func(r *kgo.Record, e error) {
		if callback != nil {
			callback(kafka.Delivery{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}, e)
		}
	})
}

// Flush waits for pending publication outcomes, bounded by context cancellation.
func (p *Producer) Flush(ctx context.Context) error { return p.client.Flush(ctx) }

// Close releases owned resources. See package documentation for shutdown ordering.
func (p *Producer) Close() error { p.client.Close(); return nil }

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (p *Producer) Native() *kgo.Client { return p.client }

// Consumer owns a Kafka consumer group client.
type Consumer struct {
	workers      int
	client       *kgo.Client
	life         consumerlife.Session
	autoCommit   bool
	commitErrors chan error
}

// NewConsumer constructs an owned consumer group client.
func NewConsumer(c Config) (*Consumer, error) {
	if c.Workers < 0 || c.Workers > 256 {
		return nil, errors.New("franz: workers must be 0..256")
	}
	if c.Workers == 0 {
		c.Workers = 1
	}
	if c.Group == "" || len(c.Topics) == 0 {
		return nil, errors.New("franz: group and topics required")
	}
	o, e := options(c)
	if e != nil {
		return nil, e
	}
	consumer := &Consumer{workers: c.Workers, autoCommit: c.AutoCommit, commitErrors: make(chan error, 1)}
	o = append(o, c.NativeOptions...)
	o = append(o, kgo.ConsumerGroup(c.Group), kgo.ConsumeTopics(c.Topics...), kgo.BlockRebalanceOnPoll())
	if c.AutoCommit {
		interval := c.CommitInterval
		if interval == 0 {
			interval = time.Second
		}
		o = append(o, kgo.AutoCommitMarks(), kgo.AutoCommitInterval(interval), kgo.AutoCommitCallback(func(_ *kgo.Client, _ *kmsg.OffsetCommitRequest, _ *kmsg.OffsetCommitResponse, e error) {
			if e != nil {
				select {
				case consumer.commitErrors <- e:
				default:
				}
			}
		}))
	} else {
		o = append(o, kgo.DisableAutoCommit())
	}
	cl, e := kgo.NewClient(o...)
	if e != nil {
		return nil, e
	}
	consumer.client = cl
	return consumer, nil
}

// Consume processes records serially, blocking rebalances only during each poll batch. Success is committed synchronously.
func (c *Consumer) Consume(ctx context.Context, h kafka.Handler) (err error) {
	if h == nil {
		return errors.New("franz: invalid handler or already consuming")
	}
	ctx, finish, e := c.life.Begin(ctx)
	if e != nil {
		return e
	}
	defer finish()
	if c.autoCommit {
		var stop func(*error)
		ctx, stop = c.watchCommits(ctx)
		defer stop(&err)
	} else {
		defer c.client.AllowRebalance()
	}
	for {
		fetches := c.client.PollRecords(ctx, 100)
		if e := ctx.Err(); e != nil {
			return e
		}
		if es := fetches.Errors(); len(es) > 0 {
			return es[0].Err
		}
		if e := c.process(ctx, fetches.Records(), h); e != nil {
			return e
		}
		c.client.AllowRebalance()
	}
}

// Pause pauses fetching for the specified scope.
func (c *Consumer) Pause(topics ...string) { c.client.PauseFetchTopics(topics...) }

// Resume resumes fetching for the specified scope.
func (c *Consumer) Resume(topics ...string) { c.client.ResumeFetchTopics(topics...) }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Consumer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.CloseContext(ctx)
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (c *Consumer) Native() *kgo.Client { return c.client }

// Admin owns a Kafka administration client.
type Admin struct{ client *kgo.Client }

// NewAdmin constructs an owned Kafka administration client.
func NewAdmin(c Config) (*Admin, error) {
	o, e := options(c)
	if e != nil {
		return nil, e
	}
	o = append(o, c.NativeOptions...)
	cl, e := kgo.NewClient(o...)
	if e != nil {
		return nil, e
	}
	return &Admin{cl}, nil
}

// CreateTopic creates a broker topic with explicit partition and replication settings.
func (a *Admin) CreateTopic(ctx context.Context, t kafka.Topic) error {
	q := kmsg.NewPtrCreateTopicsRequest()
	x := kmsg.NewCreateTopicsRequestTopic()
	x.Topic = t.Name
	x.NumPartitions = t.Partitions
	x.ReplicationFactor = t.ReplicationFactor
	for k, v := range t.Config {
		x.Configs = append(x.Configs, kmsg.CreateTopicsRequestTopicConfig{Name: k, Value: v})
	}
	q.Topics = append(q.Topics, x)
	q.TimeoutMillis = 10000
	r, e := q.RequestWith(ctx, a.client)
	if e != nil {
		return e
	}
	for _, v := range r.Topics {
		if v.ErrorCode != 0 {
			return fmt.Errorf("franz: create topic error code %d", v.ErrorCode)
		}
	}
	return nil
}

// DeleteTopic deletes the named broker topic.
func (a *Admin) DeleteTopic(ctx context.Context, name string) error {
	q := kmsg.NewPtrDeleteTopicsRequest()
	q.TopicNames = []string{name}
	q.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &name}}
	q.TimeoutMillis = 10000
	r, e := q.RequestWith(ctx, a.client)
	if e != nil {
		return e
	}
	for _, v := range r.Topics {
		if v.ErrorCode != 0 {
			return fmt.Errorf("franz: delete topic error code %d", v.ErrorCode)
		}
	}
	return nil
}

// ListTopics returns available topic names and partition IDs.
func (a *Admin) ListTopics(ctx context.Context) ([]kafka.TopicInfo, error) {
	q := kmsg.NewPtrMetadataRequest()
	r, e := q.RequestWith(ctx, a.client)
	if e != nil {
		return nil, e
	}
	var out []kafka.TopicInfo
	for _, t := range r.Topics {
		if t.ErrorCode != 0 {
			return nil, fmt.Errorf("franz: metadata error code %d", t.ErrorCode)
		}
		if t.Topic == nil {
			continue
		}
		x := kafka.TopicInfo{Name: *t.Topic, Internal: t.IsInternal}
		for _, p := range t.Partitions {
			x.Partitions = append(x.Partitions, p.Partition)
		}
		out = append(out, x)
	}
	return out, nil
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (a *Admin) Close() error { a.client.Close(); return nil }

var _ kafka.Producer = (*Producer)(nil)
var _ kafka.Consumer = (*Consumer)(nil)
var _ kafka.Admin = (*Admin)(nil)

// ConsumeAcknowledged commits only when the handler explicitly calls Ack and then returns nil.
func (c *Consumer) ConsumeAcknowledged(ctx context.Context, h kafka.AckHandler) error {
	return c.Consume(ctx, kafka.RequireAck(h))
}

var _ kafka.ManualConsumer = (*Consumer)(nil)

// PublishWithDelivery waits for the configured broker acknowledgment and returns assigned metadata.
func (p *Producer) PublishWithDelivery(ctx context.Context, m kafka.Message) (kafka.Delivery, error) {
	type outcome struct {
		delivery kafka.Delivery
		err      error
	}
	result := make(chan outcome, 1)
	p.client.Produce(ctx, record(m), func(r *kgo.Record, err error) {
		result <- outcome{kafka.Delivery{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}, err}
	})
	select {
	case v := <-result:
		return v.delivery, v.err
	case <-ctx.Done():
		return kafka.Delivery{}, ctx.Err()
	}
}

func (c *Consumer) process(ctx context.Context, records []*kgo.Record, h kafka.Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type partition struct {
		topic string
		id    int32
	}
	groups := map[partition][]*kgo.Record{}
	offsets := map[string]map[int32]kgo.EpochOffset{}
	var offsetMu sync.Mutex
	for _, r := range records {
		k := partition{r.Topic, r.Partition}
		if offsets[r.Topic] == nil {
			offsets[r.Topic] = map[int32]kgo.EpochOffset{}
		}
		if _, ok := offsets[r.Topic][r.Partition]; !ok {
			offsets[r.Topic][r.Partition] = kgo.EpochOffset{Epoch: r.LeaderEpoch, Offset: r.Offset}
		}
		groups[k] = append(groups[k], r)
	}
	sem := make(chan struct{}, c.workers)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for _, records := range groups {
		wg.Add(1)
		go func(records []*kgo.Record) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			for _, r := range records {
				if e := ctx.Err(); e != nil {
					return
				}
				e := kafka.Recover(h)(ctx, message(r))
				if e == nil {
					if c.autoCommit {
						c.client.MarkCommitRecords(r)
					} else {
						e = c.client.CommitRecords(ctx, r)
					}
				}
				if e == nil {
					offsetMu.Lock()
					offsets[r.Topic][r.Partition] = kgo.EpochOffset{Epoch: r.LeaderEpoch, Offset: r.Offset + 1}
					offsetMu.Unlock()
				}
				if e != nil {
					select {
					case errs <- e:
					default:
					}
					cancel()
					return
				}
			}
		}(records)
	}
	wg.Wait()
	select {
	case e := <-errs:
		// SetOffsets resets marked positions; persist processed marks before rewinding.
		if c.autoCommit {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			e = errors.Join(e, c.client.CommitMarkedOffsets(cleanup))
			stop()
		}
		c.client.SetOffsets(offsets)
		return e
	default:
		if ctx.Err() != nil {
			if c.autoCommit {
				cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				commitErr := c.client.CommitMarkedOffsets(cleanup)
				stop()
				c.client.SetOffsets(offsets)
				return errors.Join(ctx.Err(), commitErr)
			}
			c.client.SetOffsets(offsets)
		}
		return ctx.Err()
	}
}
