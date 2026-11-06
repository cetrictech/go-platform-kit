package instrument

import (
	"context"
	"testing"
)

func TestNoop(t *testing.T) {
	ctx, done := Start(context.Background(), nil, "op")
	done(nil)
	if ctx == nil {
		t.Fatal("context")
	}
}
