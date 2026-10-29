package retry

import (
	"errors"
	"github.com/tigusigalpa/bitfinex-go/types"
	"testing"
	"time"
)

func TestRetryClassification(t *testing.T) {
	p := DefaultPolicy()
	for _, err := range []error{&types.RateLimitError{APIError: types.APIError{StatusCode: 429}}, &types.APIError{StatusCode: 503}} {
		if !p.Retryable(err, true) || p.Retryable(err, false) {
			t.Fatal("incorrect retry classification")
		}
	}
	for _, err := range []error{&types.AuthenticationError{APIError: types.APIError{StatusCode: 503}}, &types.NonceError{}, &types.ProtocolError{}, &types.ValidationError{}, errors.New("permanent")} {
		if p.Retryable(err, true) {
			t.Fatalf("retried %T", err)
		}
	}
	for _, attempt := range []int{0, 1, 5, 64, 1000} {
		d := p.DelayForAttempt(attempt)
		if d < 0 || d > p.MaxDelay {
			t.Fatal("unbounded jitter")
		}
	}
	p.SetNow(func() time.Time { return time.Unix(100, 0) })
	if !p.ElapsedExceeded(time.Unix(1, 0)) {
		t.Fatal("elapsed budget ignored")
	}
}
