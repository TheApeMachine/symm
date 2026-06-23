package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

const traderPumpDumpWarmupTicks = 59

var traderReplayTimestamp = time.Now().UnixNano()

func traderTickerPayload(
	symbol string,
	volume float64,
	last float64,
	bid float64,
	ask float64,
) []byte {
	return fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"ask":%g,"last":%g,"volume":%g}]}`,
		symbol,
		bid,
		ask,
		last,
		volume,
	)
}

func insertTraderTicker(
	tree *dmt.Tree,
	symbol string,
	volume float64,
	last float64,
	bid float64,
	ask float64,
) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload(traderTickerPayload(symbol, volume, last, bid, ask))
	traderReplayTimestamp++
	artifact.SetTimestamp(traderReplayTimestamp)
	tree.Insert(artifact.Prefix(), artifact.Pack())
	artifact.Release()
}

func warmupTraderPumpDump(tree *dmt.Tree) {
	for tick := range traderPumpDumpWarmupTicks {
		insertTraderTicker(
			tree,
			"ETH/USD",
			100*float64(tick+1),
			10000+float64(tick)*0.1,
			9990,
			10010,
		)
	}
}

func newTraderSignal(t testing.TB) (*Signal, *dmt.Tree) {
	t.Helper()

	tree := dmt.NewTree("")
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return NewSignal(context.Background(), pool, tree), tree
}

func TestSignalMeasurePublishesPumpDumpMeasurement(t *testing.T) {
	signal, tree := newTraderSignal(t)
	defer signal.Close()

	warmupTraderPumpDump(tree)
	insertTraderTicker(tree, "ETH/USD", 11000, 41000, 40990, 41010)

	measurements := signal.Measure()

	if len(measurements) == 0 {
		t.Fatal("Measure returned no measurements")
	}

	for _, measurement := range measurements {
		role, _ := measurement.Role()
		origin, _ := measurement.Origin()
		confidence := datura.Peek[float64](measurement, "output", "confidence")
		category := datura.Peek[float64](measurement, "output", "category")

		if role != "measurement" {
			t.Fatalf("role = %q, want measurement", role)
		}

		if origin != string(logic.SourcePumpDump) {
			t.Fatalf("origin = %q, want %q", origin, logic.SourcePumpDump)
		}

		if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
			t.Fatalf("confidence = %v, want finite positive", confidence)
		}

		if category <= 0 {
			t.Fatalf("category = %v, want positive", category)
		}
	}
}

func BenchmarkSignalMeasure(b *testing.B) {
	errnie.Apply(&errnie.Config{Level: "panic"})
	b.ReportAllocs()

	for b.Loop() {
		signal, tree := newTraderSignal(b)
		warmupTraderPumpDump(tree)
		insertTraderTicker(tree, "ETH/USD", 11000, 41000, 40990, 41010)

		measurements := signal.Measure()

		if len(measurements) == 0 {
			b.Fatal("Measure returned no measurements")
		}

		_ = signal.Close()
	}
}

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
