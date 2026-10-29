// Authenticated read-only example: wallets + active orders.
// Credentials come from the environment — never hardcode them.
// Use a READ-ONLY API key for this example.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
)

func main() {
	key := os.Getenv("BFX_API_KEY")
	secret := os.Getenv("BFX_API_SECRET")
	if key == "" || secret == "" {
		log.Fatal("set BFX_API_KEY and BFX_API_SECRET (read-only key recommended)")
	}

	client, err := bitfinex.NewClient(bitfinex.WithCredentials(key, secret))
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	wallets, err := client.Wallets.All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range wallets.Data {
		fmt.Printf("wallet %s %s balance=%s available=%s\n",
			w.Type, w.Currency, w.Balance, w.AvailableBalance)
	}

	open, err := client.Orders.Active(ctx, "")
	if err != nil {
		log.Fatal(err)
	}
	for _, o := range open.Data {
		fmt.Printf("order #%d %s %s amount=%s price=%s status=%s\n",
			o.ID, o.Symbol, o.Type, o.Amount, o.Price, o.Status)
	}
}
