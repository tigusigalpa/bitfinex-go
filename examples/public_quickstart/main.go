// Public read-only quickstart — no credentials required.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
)

func main() {
	client, err := bitfinex.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status, err := client.Market.PlatformStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("platform status:", status.Data)

	ticker, err := client.Market.Ticker(ctx, "tBTCUSD")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tBTCUSD last=%s bid=%s ask=%s volume=%s\n",
		ticker.Data.LastPrice, ticker.Data.Bid, ticker.Data.Ask, ticker.Data.Volume)

	candles, err := client.Market.Candles(ctx, "trade:1h:tBTCUSD", "hist", &bitfinex.HistoryQuery{Limit: 5})
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range candles.Data {
		fmt.Printf("candle %s close=%s\n", c.MTS.Time().Format(time.RFC3339), c.Close)
	}
}
