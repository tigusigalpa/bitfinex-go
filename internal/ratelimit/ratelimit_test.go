package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestBurstThenWait(t *testing.T) {
	l := New(map[string]float64{"b": 60}, time.Hour) // 1/sec
	ctx := context.Background()
	// First 60 tokens are instant.
	for i := 0; i < 60; i++ {
		if err := l.Wait(ctx, "b"); err != nil {
			t.Fatalf("burst wait %d failed: %v", i, err)
		}
	}
	// Next requires ~1s wait — cancel it.
	cctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err := l.Wait(cctx, "b"); err == nil {
		t.Fatal("wait should have been cancelled")
	}
}

func TestCooldownAfterPenalize(t *testing.T) {
	l := New(nil, time.Hour)
	l.Penalize("wallets", 0)
	if d := l.AvailabilityDelay("wallets"); d < 55*time.Second {
		t.Fatalf("cooldown = %v, want >=55s", d)
	}
	// Longer Retry-After wins.
	l.Penalize("wallets", 120)
	if d := l.AvailabilityDelay("wallets"); d < 110*time.Second {
		t.Fatalf("cooldown = %v, want >=110s", d)
	}
}

func TestMaxWaitExceeded(t *testing.T) {
	l := New(map[string]float64{"b": 1}, 50*time.Millisecond) // 1/min
	ctx := context.Background()
	if err := l.Wait(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	var rle *types.RateLimitError
	if err := l.Wait(ctx, "b"); !errors.As(err, &rle) {
		t.Fatalf("want RateLimitError, got %v", err)
	}
}
