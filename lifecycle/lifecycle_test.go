package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRollback(t *testing.T) {
	var order []string
	m := Manager{}
	_ = m.Register(Hook{Name: "one", Start: func(context.Context) error { order = append(order, "start"); return nil }, Stop: func(context.Context) error { order = append(order, "stop"); return nil }})
	_ = m.Register(Hook{Name: "two", Start: func(context.Context) error { return errors.New("failure") }})
	if m.Start(context.Background()) == nil || !reflect.DeepEqual(order, []string{"start", "stop"}) {
		t.Fatal(order)
	}
	if m.Stop(context.Background()) != nil {
		t.Fatal("idempotent stop")
	}
}

type testResource struct{ closed *int }

func (r testResource) Close() error { *r.closed++; return nil }
func TestResourceFactoriesAndPartialCleanup(t *testing.T) {
	ctx := context.Background()
	m := Manager{}
	first, partial := 0, 0
	if e := m.RegisterResource("first", func(context.Context) (any, error) { return testResource{&first}, nil }); e != nil {
		t.Fatal(e)
	}
	if e := m.RegisterResource("partial", func(context.Context) (any, error) { return testResource{&partial}, errors.New("failed init") }); e != nil {
		t.Fatal(e)
	}
	if e := m.Start(ctx); e == nil || first != 1 || partial != 1 {
		t.Fatal(first, partial, e)
	}
	if e := m.Stop(ctx); e != nil || first != 1 || partial != 1 {
		t.Fatal(first, partial, e)
	}
}
