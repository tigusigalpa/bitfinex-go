// Package authflow orders nonce-bearing exchanges within one process. REST
// attempts and WebSocket authentication sharing an API key use the same gate.
package authflow

import (
	"context"
	"sync"
)

type entry struct {
	token chan struct{}
	users int
}

var gates = struct {
	sync.Mutex
	entries map[string]*entry
}{entries: make(map[string]*entry)}

// Acquire waits before a nonce is generated. The caller must release only after
// the HTTP response or WebSocket auth acknowledgement (or a terminal failure).
// Idle entries are removed, retaining no API keys between exchanges. This does
// not coordinate other processes or replace a shared monotonic nonce provider.
func Acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gates.Lock()
	e := gates.entries[key]
	if e == nil {
		e = &entry{token: make(chan struct{}, 1)}
		gates.entries[key] = e
	}
	e.users++
	gates.Unlock()
	unref := func() {
		gates.Lock()
		e.users--
		if e.users == 0 {
			delete(gates.entries, key)
		}
		gates.Unlock()
	}
	select {
	case e.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-e.token
			unref()
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(func() { <-e.token; unref() }) }, nil
	case <-ctx.Done():
		unref()
		return nil, ctx.Err()
	}
}
