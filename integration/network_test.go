//go:build integration

package integration

import (
	"context"
	"errors"
	"net"
	"sync"
)

// faultNetwork interrupts all connections, including addresses returned in Kafka metadata.
type faultNetwork struct {
	mu          sync.Mutex
	offline     bool
	connections map[*faultConn]bool
}
type faultConn struct {
	net.Conn
	owner *faultNetwork
}

func (c *faultConn) Close() error {
	c.owner.mu.Lock()
	delete(c.owner.connections, c)
	c.owner.mu.Unlock()
	return c.Conn.Close()
}
func (n *faultNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	n.mu.Lock()
	offline := n.offline
	n.mu.Unlock()
	if offline {
		return nil, errors.New("injected network outage")
	}
	conn, err := new(net.Dialer).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	wrapped := &faultConn{Conn: conn, owner: n}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.offline {
		conn.Close()
		return nil, errors.New("injected network outage")
	}
	if n.connections == nil {
		n.connections = map[*faultConn]bool{}
	}
	n.connections[wrapped] = true
	return wrapped, nil
}
func (n *faultNetwork) setOffline(offline bool) {
	n.mu.Lock()
	n.offline = offline
	var connections []*faultConn
	if offline {
		for c := range n.connections {
			connections = append(connections, c)
		}
	}
	n.mu.Unlock()
	for _, c := range connections {
		c.Close()
	}
}
