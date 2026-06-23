package trader

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func insertTraderInstrument(tree *dmt.Tree) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("instrument")
	artifact.WithScope("snapshot")
	artifact.WithPayload([]byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"BTC/USD","quote":"USD"},{"symbol":"ETH/USD","quote":"USD"},{"symbol":"ETH/EUR","quote":"EUR"}]}}`))
	tree.Insert(artifact.Prefix(), artifact.Pack())
	artifact.Release()
}

func TestCryptoSubscribeToStreamsPublishesSymbolSubscriptions(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	oldLimit := viper.GetInt("market.max_scan_symbols")

	viper.Set("market.quote_currency", "USD")
	viper.Set("market.max_scan_symbols", 2)

	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
		viper.Set("market.max_scan_symbols", oldLimit)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	tree := dmt.NewTree("")
	pool := qpool.NewQ[any](ctx, 2, 4, nil)
	consumer := pool.Subscribe("kraken:public", nil)
	broadcasts := &sync.Map{}
	broadcasts.Store("kraken:public", pool.CreateBroadcastGroup("kraken:public"))

	crypto := &Crypto{
		tree:       tree,
		pool:       pool,
		broadcasts: broadcasts,
		pairs:      &sync.Map{},
	}

	insertTraderInstrument(tree)

	if err := crypto.subscribeToStreams(); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}

	for range 4 {
		artifact, err := consumer.Wait(ctx)

		if err != nil {
			t.Fatal(err)
		}

		var frame struct {
			Method string `json:"method"`
			Params struct {
				Channel  string   `json:"channel"`
				Snapshot bool     `json:"snapshot"`
				Symbol   []string `json:"symbol"`
			} `json:"params"`
		}

		if err := json.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
			t.Fatal(err)
		}

		if frame.Method != "subscribe" {
			t.Fatalf("method = %q, want subscribe", frame.Method)
		}

		if !frame.Params.Snapshot {
			t.Fatal("snapshot = false, want true")
		}

		if len(frame.Params.Symbol) != 2 {
			t.Fatalf("symbols = %v, want BTC/USD and ETH/USD", frame.Params.Symbol)
		}

		if frame.Params.Symbol[0] != "BTC/USD" || frame.Params.Symbol[1] != "ETH/USD" {
			t.Fatalf("symbols = %v, want BTC/USD and ETH/USD", frame.Params.Symbol)
		}

		seen[frame.Params.Channel] = true
	}

	for _, stream := range []string{"ohlc", "ticker", "book", "trade"} {
		if !seen[stream] {
			t.Fatalf("missing %s subscribe", stream)
		}
	}
}
