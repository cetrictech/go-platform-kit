//go:build integration

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecureKafkaInteroperability(t *testing.T) {
	secure := testTLS(t)
	for _, bits := range []int{256, 512} {
		for _, producer := range []string{"franz", "sarama"} {
			t.Run(fmt.Sprintf("SCRAM%d-%s", bits, producer), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				topic := fmt.Sprintf("gpk-secure-%d-%s-%d", bits, producer, time.Now().UnixNano())
				provider := func(ctx context.Context) (kafka.Credentials, error) {
					if err := ctx.Err(); err != nil {
						return kafka.Credentials{}, err
					}
					return kafka.Credentials{Username: fmt.Sprintf("sdk-%d", bits), Password: "sdk-test-only"}, nil
				}
				fc := f.Config{Brokers: []string{"127.0.0.1:29092"}, TLS: secure, SASL: fmt.Sprintf("SCRAM-SHA-%d", bits), Credentials: provider, Topics: []string{topic}, Group: topic}
				sc := s.Config{Brokers: fc.Brokers, TLS: secure, SASL: fc.SASL, Credentials: provider, Topics: fc.Topics, Group: topic}
				admin, err := f.NewAdmin(fc)
				if err != nil {
					t.Fatal(err)
				}
				defer admin.Close()
				if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
					t.Fatal(err)
				}
				defer admin.DeleteTopic(context.Background(), topic)
				var p kafka.Producer
				var c kafka.Consumer
				if producer == "franz" {
					p, err = f.NewProducer(fc)
					if err == nil {
						c, err = s.NewConsumer(sc)
					}
				} else {
					p, err = s.NewProducer(sc)
					if err == nil {
						c, err = f.NewConsumer(fc)
					}
				}
				if p != nil {
					defer p.Close()
				}
				if c != nil {
					defer c.Close()
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := p.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("authenticated")}); err != nil {
					t.Fatal(err)
				}
				stop := errors.New("verified delivery")
				if err := c.Consume(ctx, func(ctx context.Context, m kafka.Message) error {
					if string(m.Value) != "authenticated" {
						return errors.New("unexpected payload")
					}
					return stop
				}); !errors.Is(err, stop) {
					t.Fatal(err)
				}
			})
		}
	}
}

func testTLS(t *testing.T) *tls.Config {
	t.Helper()
	dir := os.Getenv("GPK_TLS_DIR")
	if dir == "" {
		t.Skip("GPK_TLS_DIR unset")
	}
	pem, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("invalid CA")
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key"))
	if err != nil {
		t.Fatal(err)
	}
	secure := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}}

	return secure
}
