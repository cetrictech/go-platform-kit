package sarama

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
)

// DescribeTopic returns topic metadata and partition placement.
func (a *Admin) DescribeTopic(ctx context.Context, name string) (kafka.TopicDescription, error) {
	if e := ctx.Err(); e != nil {
		return kafka.TopicDescription{}, e
	}
	topics, e := a.admin.DescribeTopics([]string{name})
	if e != nil {
		return kafka.TopicDescription{}, e
	}
	if len(topics) != 1 {
		return kafka.TopicDescription{}, errors.New("sarama: topic not found")
	}
	t := topics[0]
	if t.Err != 0 {
		return kafka.TopicDescription{}, t.Err
	}
	out := kafka.TopicDescription{Name: t.Name, Internal: t.IsInternal}
	for _, p := range t.Partitions {
		if p.Err != 0 {
			return out, p.Err
		}
		out.Partitions = append(out.Partitions, kafka.PartitionInfo{ID: p.ID, Leader: p.Leader, Replicas: p.Replicas, InSyncReplicas: p.Isr})
	}
	return out, nil
}

// ListGroups lists consumer groups.
func (a *Admin) ListGroups(ctx context.Context) ([]kafka.GroupInfo, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	groups, e := a.admin.ListConsumerGroups()
	if e != nil {
		return nil, e
	}
	out := make([]kafka.GroupInfo, 0, len(groups))
	for name, protocol := range groups {
		out = append(out, kafka.GroupInfo{Name: name, ProtocolType: protocol})
	}
	return out, nil
}

// DescribeGroup returns consumer group state and membership.
func (a *Admin) DescribeGroup(ctx context.Context, name string) (kafka.GroupInfo, error) {
	if e := ctx.Err(); e != nil {
		return kafka.GroupInfo{}, e
	}
	groups, e := a.admin.DescribeConsumerGroups([]string{name})
	if e != nil {
		return kafka.GroupInfo{}, e
	}
	if len(groups) != 1 {
		return kafka.GroupInfo{}, errors.New("sarama: group not found")
	}
	g := groups[0]
	if g.Err != 0 {
		return kafka.GroupInfo{}, g.Err
	}
	out := kafka.GroupInfo{Name: g.GroupId, State: g.State, ProtocolType: g.ProtocolType}
	for _, m := range g.Members {
		out.Members = append(out.Members, kafka.GroupMember{ID: m.MemberId, ClientID: m.ClientId, Host: m.ClientHost})
	}
	return out, nil
}

// Offsets returns committed resume offsets for a consumer group.
func (a *Admin) Offsets(ctx context.Context, group string) ([]kafka.Offset, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	r, e := a.admin.ListConsumerGroupOffsets(group, nil)
	if e != nil {
		return nil, e
	}
	if r.Err != 0 {
		return nil, r.Err
	}
	var out []kafka.Offset
	for topic, partitions := range r.Blocks {
		for partition, o := range partitions {
			if o.Err != 0 {
				return nil, o.Err
			}
			out = append(out, kafka.Offset{Topic: topic, Partition: partition, Offset: o.Offset, Metadata: o.Metadata})
		}
	}
	return out, nil
}

var _ kafka.Inspector = (*Admin)(nil)
