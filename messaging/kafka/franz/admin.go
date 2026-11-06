package franz

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"github.com/twmb/franz-go/pkg/kadm"
)

// DescribeTopic returns topic metadata and partition placement.
func (a *Admin) DescribeTopic(ctx context.Context, name string) (kafka.TopicDescription, error) {
	topics, e := kadm.NewClient(a.client).ListTopicsWithInternal(ctx, name)
	if e != nil {
		return kafka.TopicDescription{}, e
	}
	t, ok := topics[name]
	if !ok {
		return kafka.TopicDescription{}, errors.New("franz: topic not found")
	}
	if t.Err != nil {
		return kafka.TopicDescription{}, t.Err
	}
	out := kafka.TopicDescription{Name: t.Topic, Internal: t.IsInternal}
	for _, p := range t.Partitions {
		if p.Err != nil {
			return out, p.Err
		}
		out.Partitions = append(out.Partitions, kafka.PartitionInfo{ID: p.Partition, Leader: p.Leader, Replicas: p.Replicas, InSyncReplicas: p.ISR})
	}
	return out, nil
}

// ListGroups lists consumer groups.
func (a *Admin) ListGroups(ctx context.Context) ([]kafka.GroupInfo, error) {
	groups, e := kadm.NewClient(a.client).ListGroups(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]kafka.GroupInfo, 0, len(groups))
	for _, g := range groups {
		out = append(out, kafka.GroupInfo{Name: g.Group, State: g.State, ProtocolType: g.ProtocolType})
	}
	return out, nil
}

// DescribeGroup returns consumer group state and membership.
func (a *Admin) DescribeGroup(ctx context.Context, name string) (kafka.GroupInfo, error) {
	groups, e := kadm.NewClient(a.client).DescribeGroups(ctx, name)
	if e != nil {
		return kafka.GroupInfo{}, e
	}
	g, ok := groups[name]
	if !ok {
		return kafka.GroupInfo{}, errors.New("franz: group not found")
	}
	if g.Err != nil {
		return kafka.GroupInfo{}, g.Err
	}
	out := kafka.GroupInfo{Name: g.Group, State: g.State, ProtocolType: g.ProtocolType}
	for _, m := range g.Members {
		out.Members = append(out.Members, kafka.GroupMember{ID: m.MemberID, ClientID: m.ClientID, Host: m.ClientHost})
	}
	return out, nil
}

// Offsets returns committed resume offsets for a consumer group.
func (a *Admin) Offsets(ctx context.Context, group string) ([]kafka.Offset, error) {
	offsets, e := kadm.NewClient(a.client).FetchOffsets(ctx, group)
	if e != nil {
		return nil, e
	}
	var out []kafka.Offset
	for topic, partitions := range offsets {
		for partition, o := range partitions {
			if o.Err != nil {
				return nil, o.Err
			}
			out = append(out, kafka.Offset{Topic: topic, Partition: partition, Offset: o.At, Metadata: o.Metadata})
		}
	}
	return out, nil
}

var _ kafka.Inspector = (*Admin)(nil)
