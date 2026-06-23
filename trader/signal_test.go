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

func insertTraderTickerAt(
	tree *dmt.Tree,
	symbol string,
	timestamp int64,
	uuid string,
) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.SetTimestamp(timestamp)
	artifact.SetUuid([]byte(uuid))
	artifact.WithPayload(traderTickerPayload(symbol, 100, 100, 99, 101))
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

type recordingSignal struct {
	timestamps []int64
}

func (recording *recordingSignal) Measure(artifact *datura.Artifact) *datura.Artifact {
	recording.timestamps = append(recording.timestamps, artifact.Timestamp())

	return nil
}

func (recording *recordingSignal) IngestRoles() []string {
	return []string{"ticker"}
}

func (recording *recordingSignal) Close() error {
	return nil
}

func TestSignalMeasureSortsRetrievedArtifactsByTimestamp(t *testing.T) {
	tree := dmt.NewTree("")
	recorder := &recordingSignal{}
	signal := &Signal{
		tree: tree,
		signals: []wiredSignal{
			{measurer: recorder, origin: logic.SourcePumpDump},
		},
	}

	insertTraderTickerAt(tree, "EUR/USD", 200, "a-newer")
	insertTraderTickerAt(tree, "EUR/USD", 100, "z-older")

	signal.Measure()

	if len(recorder.timestamps) != 2 {
		t.Fatalf("timestamps = %v, want two measurements", recorder.timestamps)
	}

	if recorder.timestamps[0] != 100 || recorder.timestamps[1] != 200 {
		t.Fatalf("timestamps = %v, want [100 200]", recorder.timestamps)
	}
}

func TestSignalMeasureAdvancesCursor(t *testing.T) {
	tree := dmt.NewTree("")
	recorder := &recordingSignal{}
	signal := &Signal{
		tree: tree,
		signals: []wiredSignal{
			{measurer: recorder, origin: logic.SourcePumpDump},
		},
	}

	insertTraderTickerAt(tree, "EUR/USD", 100, "a-first")
	insertTraderTickerAt(tree, "EUR/USD", 200, "b-second")

	signal.Measure()
	signal.Measure()

	if len(recorder.timestamps) != 2 {
		t.Fatalf("timestamps = %v, want first pass only", recorder.timestamps)
	}

	insertTraderTickerAt(tree, "EUR/USD", 300, "c-third")

	signal.Measure()

	if len(recorder.timestamps) != 3 {
		t.Fatalf("timestamps = %v, want one new measurement", recorder.timestamps)
	}

	if recorder.timestamps[2] != 300 {
		t.Fatalf("timestamps = %v, want third timestamp 300", recorder.timestamps)
	}
}

func TestNewSignalWiresSpectrumSignals(t *testing.T) {
	signal, _ := newTraderSignal(t)
	defer signal.Close()

	expected := map[logic.SourceType]bool{
		logic.SourceHawkes:      false,
		logic.SourceFluid:       false,
		logic.SourcePumpDump:    false,
		logic.SourceCausal:      false,
		logic.SourceDepthFlow:   false,
		logic.SourceLeadLag:     false,
		logic.SourceLiquidity:   false,
		logic.SourceSentiment:   false,
		logic.SourceToxicity:    false,
		logic.SourceCorrelation: false,
		logic.SourceExhaustion:  false,
		logic.SourceCVD:         false,
		logic.SourceManifold:    false,
	}

	if len(signal.signals) != len(expected) {
		t.Fatalf("wired signals = %d, want %d", len(signal.signals), len(expected))
	}

	for _, wired := range signal.signals {
		if _, ok := expected[wired.origin]; !ok {
			t.Fatalf("unexpected signal origin %q", wired.origin)
		}

		if len(wired.measurer.IngestRoles()) == 0 {
			t.Fatalf("signal %q has no ingest roles", wired.origin)
		}

		expected[wired.origin] = true
	}

	for source, seen := range expected {
		if !seen {
			t.Fatalf("signal %q was not wired", source)
		}
	}
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

	foundPumpDump := false

	for _, measurement := range measurements {
		role, _ := measurement.Role()
		origin, _ := measurement.Origin()

		if origin != string(logic.SourcePumpDump) {
			continue
		}

		foundPumpDump = true
		confidence := datura.Peek[float64](measurement, "output", "confidence")
		category := datura.Peek[float64](measurement, "output", "category")

		if role != "measurement" {
			t.Fatalf("role = %q, want measurement", role)
		}

		if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
			t.Fatalf("confidence = %v, want finite positive", confidence)
		}

		if category <= 0 {
			t.Fatalf("category = %v, want positive", category)
		}
	}

	if !foundPumpDump {
		t.Fatal("Measure returned no pumpdump measurement")
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
