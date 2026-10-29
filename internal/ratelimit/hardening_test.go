package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCooldownAppliesAcrossBucketsAndNeverShrinks(t *testing.T) {
	l := New(nil, time.Hour)
	l.Penalize("wallets", 120)
	l.Penalize("wallets", 60)
	if d := l.AvailabilityDelay("ticker"); d < 110*time.Second {
		t.Fatal("IP cooldown missing on another endpoint or was shortened")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx, "ticker"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("another endpoint bypassed IP cooldown: %v", err)
	}
}

func TestConcurrentWaitersRecheckTokenAvailability(t *testing.T) {
	l := New(map[string]float64{"test": 6000}, time.Second)
	l.buckets["test"] = &bucket{tokens: 0, started: true, refilledAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Wait(ctx, "test"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
