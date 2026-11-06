package postgres

import (
	"context"
	"errors"
	"sync/atomic"
)

// ClusterConfig configures independently owned primary/replica pools. Replicas are explicit and are never guessed from SQL.
type ClusterConfig struct {
	Primary  Config
	Replicas []Config
}

// Consistency selects primary freshness or potentially stale replica reads.
type Consistency int

const (
	Strong Consistency = iota
	Eventual
)

// Cluster owns all configured clients. Writes/transactions belong on Writer; Reader is an explicit consistency choice.
type Cluster struct {
	primary  *Client
	replicas []*Client
	next     atomic.Uint64
}

// NewCluster opens pools atomically, cleaning up previously opened pools on any initialization failure.
func NewCluster(ctx context.Context, cfg ClusterConfig) (*Cluster, error) {
	primary, e := New(ctx, cfg.Primary)
	if e != nil {
		return nil, e
	}
	c := &Cluster{primary: primary}
	for _, cfg := range cfg.Replicas {
		cfg.ReadOnly = true
		r, e := New(ctx, cfg)
		if e != nil {
			c.Close()
			return nil, e
		}
		c.replicas = append(c.replicas, r)
	}
	return c, nil
}

// Writer returns the primary client for writes and read-after-write consistency.
func (c *Cluster) Writer() *Client { return c.primary }

// Reader rotates replicas for Eventual, falling back to primary when none are configured. Strong always uses primary.
// Replication lag, concurrent writes and failover can invalidate read-after-write assumptions on replicas.
func (c *Cluster) Reader(consistency Consistency) (*Client, error) {
	switch consistency {
	case Strong:
		return c.primary, nil
	case Eventual:
		if len(c.replicas) == 0 {
			return c.primary, nil
		}
		return c.replicas[(c.next.Add(1)-1)%uint64(len(c.replicas))], nil
	default:
		return nil, errors.New("postgres: unknown read consistency")
	}
}

// Close closes replicas in reverse order, then the primary. Drain callers before closing.
func (c *Cluster) Close() {
	for i := len(c.replicas) - 1; i >= 0; i-- {
		c.replicas[i].Close()
	}
	c.primary.Close()
}
