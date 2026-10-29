// Public WebSocket quickstart: subscribe to the tBTCUSD ticker
// and print updates until interrupted or timeout.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/websocket"
)

func main() {
	client, err := bitfinex.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	ws := client.NewWebSocket()
	defer ws.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ws.ConnectPublic(ctx, 0); err != nil {
		log.Fatal(err)
	}

	sub, err := ws.Subscribe(ctx, websocket.SubscribeTicker("tBTCUSD"))
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.Events():
			if !ok {
				return
			}
			if ev.Kind == websocket.KindUpdate && ev.Ticker != nil {
				fmt.Printf("ticker %s last=%s bid=%s ask=%s\n",
					ev.Symbol, ev.Ticker.LastPrice, ev.Ticker.Bid, ev.Ticker.Ask)
			}
			if ev.Kind == websocket.KindError {
				log.Printf("ws error: %v", ev.Err)
				return
			}
		}
	}
}
