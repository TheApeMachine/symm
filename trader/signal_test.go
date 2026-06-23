package trader

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
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

type blockingSignal struct {
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (blocking *blockingSignal) Measure(artifact *datura.Artifact) *datura.Artifact {
	blocking.calls++

	if blocking.calls == 2 {
		close(blocking.entered)
		<-blocking.release
	}

	return artifact
}

func (blocking *blockingSignal) IngestRoles() []string {
	return []string{"ticker"}
}

func (blocking *blockingSignal) Close() error {
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

func TestCryptoRunPublishesMeasurementBeforeSweepCompletes(t *testing.T) {
	oldInterval := viper.Get("market.story.ui_interval")
	viper.Set("market.story.ui_interval", time.Millisecond)
	t.Cleanup(func() {
		viper.Set("market.story.ui_interval", oldInterval)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")
	uiConsumer := pool.Subscribe("ui", nil)
	blocking := &blockingSignal{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		tree:        tree,
		pool:        pool,
		uiBroadcast: pool.CreateBroadcastGroup("ui"),
		broadcasts:  &sync.Map{},
		signals: &Signal{
			tree: tree,
			signals: []wiredSignal{
				{measurer: blocking, origin: logic.SourcePumpDump},
			},
		},
		pairs: &sync.Map{},
	}

	insertTraderTickerAt(tree, "EUR/USD", 100, "a-first")
	insertTraderTickerAt(tree, "EUR/USD", 200, "b-second")

	done := make(chan error, 1)

	go func() {
		done <- crypto.Run()
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	artifact, err := uiConsumer.Wait(waitCtx)

	if err != nil {
		t.Fatal(err)
	}

	if artifact.Timestamp() != 100 {
		t.Fatalf("published timestamp = %d, want first artifact", artifact.Timestamp())
	}

	role, _ := artifact.Role()
	origin, _ := artifact.Origin()

	if role != "measurement" || origin != string(logic.SourcePumpDump) {
		t.Fatalf("published role/origin = %q/%q, want measurement/%s", role, origin, logic.SourcePumpDump)
	}

	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("blocking signal did not reach second artifact")
	}

	close(blocking.release)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("crypto run did not stop")
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
