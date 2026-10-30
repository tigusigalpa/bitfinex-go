package bitfinex_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestDerivativeUpdateRESTExactBodyAndNoWriteRetry(t *testing.T) {
	for _, status := range []int{200, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/v2/auth/w/order/update" {
					t.Errorf("wrong route: %s %s", r.Method, r.URL.Path)
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				const want = `{"id":42,"lev":25,"price":"123.45"}`
				if string(raw) != want {
					t.Errorf("wrong update body: %s", raw)
				}
				w.WriteHeader(status)
				if status == 200 {
					fmt.Fprint(w, `[1700000000000,"ou-req",null,null,[`+orderRow+`],null,"SUCCESS","accepted"]`)
				} else {
					fmt.Fprint(w, `temporary failure`)
				}
			}))
			defer srv.Close()
			c, err := bitfinex.NewClient(bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}), bitfinex.WithRetryPolicy(3, time.Millisecond, time.Millisecond, time.Second))
			if err != nil {
				t.Fatal(err)
			}
			lev := int64(25)
			price := types.MustDecimal("123.4500")
			r, err := c.Orders.Update(context.Background(), orders.UpdateRequest{ID: 42, Leverage: &lev, Price: &price})
			if status == 200 {
				if err != nil || !r.Data.Success() {
					t.Fatalf("valid update rejected: %+v %v", r, err)
				}
			} else if err == nil {
				t.Fatal("server failure ignored")
			}
			if calls.Load() != 1 {
				t.Fatal("update retried")
			}
			lev = 101
			var ve *types.ValidationError
			if _, err = c.Orders.Update(context.Background(), orders.UpdateRequest{ID: 42, Leverage: &lev}); !errors.As(err, &ve) {
				t.Fatal("invalid leverage reached HTTP")
			}
			if calls.Load() != 1 {
				t.Fatal("invalid leverage sent a request")
			}
		})
	}
}

func TestMalformedExecutionFieldsPropagateThroughREST(t *testing.T) {
	for _, index := range []int{1, 2, 4, 5} {
		a := jsonTree(t, []byte(`[42,"tBTCUSD",100,43,1,100,"LIMIT",100,1,null,null]`)).([]any)
		a[index] = nil
		raw, err := json.Marshal([]any{a})
		if err != nil {
			t.Fatal(err)
		}
		doer := &testutil.FakeDoer{Responses: []*http.Response{testutil.JSONResponse(200, string(raw), nil)}}
		c, err := bitfinex.NewClient(bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithHTTPDoer(doer), bitfinex.WithRateLimiter(unlimited{}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Orders.Trades(context.Background(), "tBTCUSD", nil)
		var pe *types.ProtocolError
		if !errors.As(err, &pe) {
			t.Fatalf("field %d accepted through REST: %v", index, err)
		}
		if len(doer.Requests) != 1 {
			t.Fatal("protocol failure retried")
		}
	}
}
