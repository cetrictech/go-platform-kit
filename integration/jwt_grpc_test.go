//go:build integration

package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/cetrictech/go-platform-kit/grpcx"
	jwt "github.com/cetrictech/go-platform-kit/security/jwt"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"testing"
	"time"
)

func TestJWTGRPCUnaryAndStreamingAuthentication(t *testing.T) {
	key := []byte("test-only-hmac-key-at-least-32-bytes")
	verifier, err := jwt.New(context.Background(), jwt.Config{Issuer: "sdk-test", Audience: []string{"service"}, Algorithms: []string{"HS256"}, Keys: map[string]any{"test": key}})
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	server, err := grpcx.NewServer(grpcx.ServerConfig{AllowPlaintext: true, Authenticate: grpcx.BearerAuthentication(verifier.Authenticate), Authorize: func(ctx context.Context, method string) error {
		claims, ok := jwt.FromContext(ctx)
		if !ok || claims["sub"] != "alice" {
			return errors.New("missing verified identity")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(listener)
	defer server.GRPC.Stop()
	client, err := grpcx.NewClient(grpcx.ClientConfig{Target: listener.Addr().String(), AllowPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	token := func(expiry time.Time) string {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","kid":"test"}`))
		body, err := json.Marshal(map[string]any{"iss": "sdk-test", "aud": "service", "sub": "alice", "exp": expiry.Unix()})
		if err != nil {
			t.Fatal(err)
		}
		payload := header + "." + base64.RawURLEncoding.EncodeToString(body)
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(payload))
		return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	hc := healthpb.NewHealthClient(client)
	valid := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token(time.Now().Add(time.Minute)))
	if _, err := hc.Check(valid, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	stream, err := hc.Watch(valid, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	invalid := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token(time.Now().Add(-time.Minute)))
	if _, err := hc.Check(invalid, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	bad, err := hc.Watch(invalid, &healthpb.HealthCheckRequest{})
	if err == nil {
		_, err = bad.Recv()
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
}
