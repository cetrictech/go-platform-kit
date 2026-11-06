package spiffe

import (
	"context"
	"crypto/tls"

	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// PeerID returns the verified SPIFFE ID from a mutual TLS connection state.
func PeerID(state tls.ConnectionState) (string, bool) {
	if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return "", false
	}
	id, e := x509svid.IDFromCert(state.VerifiedChains[0][0])
	if e != nil {
		return "", false
	}
	return id.String(), true
}

// IDFromContext returns the verified peer SPIFFE ID from gRPC transport credentials.
func IDFromContext(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return "", false
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", false
	}
	return PeerID(info.State)
}
