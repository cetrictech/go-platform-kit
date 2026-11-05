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
