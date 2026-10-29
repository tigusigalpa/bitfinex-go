package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

const testSecret = "test-api-secret-0123456789abcdef"

func testCreds(t *testing.T) Credentials {
	t.Helper()
	c, err := NewCredentials("test-api-key-abcdef", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Golden vectors computed independently (HMAC-SHA384 over the
// exact transmitted byte string).
func TestSignGolden(t *testing.T) {
	s := NewSigner(testCreds(t))
	tests := []struct {
		apiPath, nonce, body, want string
	}{
		{
			"v2/auth/r/wallets", "1700000000001", `{}`,
			"1c8938c5319940707a1f52730c9f40754d03b5f10e7825b7a8588b7787c0fa1f01b4f69aaf145a3b43d9d991d48bbf73",
		},
		{
			"v2/auth/w/order/submit", "1700000000002",
			`{"type":"EXCHANGE LIMIT","symbol":"tBTCUSD","amount":"0.01","price":"30000"}`,
			"ee21e0685306e5aecfc48b27bdd39547b63f48beed07e18fa2518856ff61bb91100208db28d60120ecec9ceec08ae744",
		},
	}
	for _, tc := range tests {
		got, err := s.Sign(tc.apiPath, tc.nonce, []byte(tc.body))
		if err != nil {
			t.Fatalf("Sign(%s): %v", tc.apiPath, err)
		}
		if got.Signature != tc.want {
			t.Errorf("Sign(%s) = %s, want %s", tc.apiPath, got.Signature, tc.want)
		}
		if got.Nonce != tc.nonce || got.Key != "test-api-key-abcdef" {
			t.Errorf("unexpected signature set: %+v", got)
		}
	}
}

func TestSignRejectsBadPath(t *testing.T) {
	s := NewSigner(testCreds(t))
	for _, p := range []string{"", "/v2/auth/r/wallets"} {
		if _, err := s.Sign(p, "1", []byte(`{}`)); err == nil {
			t.Errorf("Sign(%q) should fail", p)
		}
	}
}

func TestSignWebSocketAuthGolden(t *testing.T) {
	s := NewSigner(testCreds(t))
	got := s.SignWebSocketAuth("1700000000003")
	want := "f79dc3e238eeb7da212253ed9a5caaa8fe1dfc0a3344956aa2a7703efb46d4f5bb6104ccf87f3277d9b8b66b4ee0c6b3"
	if got != want {
		t.Fatalf("SignWebSocketAuth = %s, want %s", got, want)
	}
	if AuthPayload("123") != "AUTH123" {
		t.Fatal("AuthPayload format")
	}
}

func TestCredentialsRedacted(t *testing.T) {
	c := testCreds(t)
	if s := c.String(); s == c.Secret() || s == c.Key() {
		t.Fatal("credentials leaked through String()")
	}
	if _, err := c.MarshalJSON(); err == nil {
		t.Fatal("credentials must not be JSON-serializable")
	}
	if _, err := NewCredentials("", "x"); err == nil {
		t.Fatal("empty key must be rejected")
	}
	if _, err := NewCredentials(" padded ", "x"); err == nil {
		t.Fatal("whitespace-padded key must be rejected")
	}
}

func TestNonceMonotonic(t *testing.T) {
	p := NewMillisecondNonceProvider(nil)
	prev := int64(0)
	for i := 0; i < 1000; i++ {
		s, err := p.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var n int64
		if _, err := parseInt(s); err != nil {
			t.Fatalf("nonce %q not numeric: %v", s, err)
		} else {
			n, _ = parseInt(s)
		}
		if n <= prev {
			t.Fatalf("nonce not strictly increasing: %d <= %d", n, prev)
		}
		if n > MaxNonce {
			t.Fatalf("nonce exceeds max: %d", n)
		}
		prev = n
	}
}

func TestNonceConcurrent(t *testing.T) {
	p := NewMillisecondNonceProvider(nil)
	const goroutines = 32
	const perG = 200
	seen := sync.Map{}
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				s, err := p.Next(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if _, dup := seen.LoadOrStore(s, true); dup {
					t.Errorf("duplicate nonce %s", s)
				}
			}
		}()
	}
	wg.Wait()
}

// fixedClock makes nonce deterministically stale.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestNonceStaleClockStillMonotonic(t *testing.T) {
	p := NewMillisecondNonceProvider(fixedClock{t: time.UnixMilli(1000)})
	a, _ := p.Next(context.Background())
	b, _ := p.Next(context.Background())
	if a != "1000" || b != "1001" {
		t.Fatalf("expected 1000→1001, got %s→%s", a, b)
	}
}

func parseInt(s string) (int64, error) {
	var v int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, context.DeadlineExceeded
		}
		v = v*10 + int64(ch-'0')
	}
	return v, nil
}
