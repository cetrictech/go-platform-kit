package mysql

import (
	"context"
	"testing"
)

func TestInvalid(t *testing.T) {
	if _, e := New(context.Background(), Config{}); e == nil {
		t.Fatal("invalid config accepted")
	}
}
