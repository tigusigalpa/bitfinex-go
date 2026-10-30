package authflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGateIsolationCancellationAndCleanup(t *testing.T) {
	release, e := Acquire(context.Background(), "test-key-one")
	if e != nil {
		t.Fatal(e)
	}
	other, e := Acquire(context.Background(), "test-key-two")
	if e != nil {
		t.Fatal(e)
	}
	other()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Acquire(ctx, "test-key-one"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e = Acquire(ctx, "test-key-one"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	release()
	release()
	again, e := Acquire(context.Background(), "test-key-one")
	if e != nil {
		t.Fatal(e)
	}
	again()
	gates.Lock()
	defer gates.Unlock()
	if len(gates.entries) != 0 {
		t.Fatal("idle keys retained")
	}
}
