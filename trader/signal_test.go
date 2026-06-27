package trader

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

type recordingSignal struct{}

func (recordingSignal) IngestRoles() []string {
	return []string{"ticker"}
}

func (recordingSignal) Measure(*datura.Artifact, *market.CrossSection) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		measurement := datura.Acquire("recording", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		measurement.WithPayload([]byte(`{"ok":true}`))
		measurement.SetTimestamp(time.Now().UTC().UnixNano())
		yield(measurement)
	}
}

func (recordingSignal) Close() error {
	return nil
}

type blockingSignal struct {
	started chan<- struct{}
	release <-chan struct{}
	scope   string
}

func (blockingSignal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal blockingSignal) Measure(*datura.Artifact, *market.CrossSection) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		signal.started <- struct{}{}
		<-signal.release

		measurement := datura.Acquire("blocking", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope(signal.scope)
		measurement.WithPayload([]byte(`{"ok":true}`))
		measurement.SetTimestamp(time.Now().UTC().UnixNano())
		yield(measurement)
	}
}

func (blockingSignal) Close() error {
	return nil
}

func TestSignalMeasureSeekPrefix(t *testing.T) {
	Convey("Given ingest keyed by timestamp like the websocket", t, func() {
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		at := time.Now().UTC().Truncate(time.Second)
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5},{"symbol":"ETH/USD","last":50,"volume":3,"change_pct":0.2,"bid":49.5,"ask":50.5},{"symbol":"SOL/USD","last":10,"volume":1,"change_pct":0.1,"bid":9.9,"ask":10.1}]}`))
		artifact.SetTimestamp(at.UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()

		runner.Observe(crossSection)
		runner.Measure(crossSection)

		Convey("It should replay the ticker frame", func() {
			So(runner.RoleCount("ticker"), ShouldEqual, 1)
			So(runner.lastTimestamp, ShouldEqual, at.UnixNano())
		})
	})
}

// TestSignalMeasureAdvancesAcrossSeconds is the regression guard for the live
// freeze: tree keys stamp time only to the second, so seeking with a
// second-granular key (role/.../HH/MM/SS) makes tree.Seek's prefix match stop at
// the end of that one second and never reach later seconds. Once the cursor
// landed in a second it could never advance, and every later frame was invisible
// no matter how much data Kraken streamed. Insert frames in DISTINCT seconds and
// require each later one to replay after the cursor moved past the earlier.
func TestSignalMeasureAdvancesAcrossSeconds(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	base := time.Now().UTC().Add(-3 * time.Second).Truncate(time.Second)
	payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`)

	insertAt := func(at time.Time) {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload(payload)
		artifact.SetTimestamp(at.UnixNano())
		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	// First second.
	insertAt(base)
	runner.Observe(crossSection)
	runner.Measure(crossSection)

	if runner.lastTimestamp != base.UnixNano() {
		t.Fatalf("cursor did not advance to first frame: got %d want %d", runner.lastTimestamp, base.UnixNano())
	}

	// A frame two seconds later: its key prefix differs from the cursor's second,
	// which is exactly the case the old prefix-seek could never reach.
	later := base.Add(2 * time.Second)
	insertAt(later)
	runner.Observe(crossSection)
	runner.Measure(crossSection)

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("later-second frame was not replayed (cursor stuck): role count %d", runner.RoleCount("ticker"))
	}

	if runner.lastTimestamp != later.UnixNano() {
		t.Fatalf("cursor did not advance across the second boundary: got %d want %d", runner.lastTimestamp, later.UnixNano())
	}
}

func TestSignalMeasureUsesBoundedSecondPrefixesAfterCursor(t *testing.T) {
	prev := time.Date(2026, 6, 26, 20, 21, 22, 500_000_000, time.UTC).UnixNano()
	prefixes := roleSeekPrefixes(
		"ticker",
		prev,
		time.Date(2026, 6, 26, 20, 21, 23, 100_000_000, time.UTC),
	)

	if len(prefixes) == 0 {
		t.Fatal("expected bounded second prefixes")
	}

	for _, prefix := range prefixes {
		if string(prefix) == "ticker/" {
			t.Fatal("cursor-backed scan fell back to full role prefix")
		}
	}

	want := map[string]bool{
		"ticker/2026/06/26/20/21/22/": true,
		"ticker/2026/06/26/20/21/23/": true,
	}

	for _, prefix := range prefixes {
		delete(want, string(prefix))
	}

	if len(want) != 0 {
		t.Fatalf("missing expected second prefixes: %#v", want)
	}
}

func TestSignalMeasureEmptyPassDoesNotAdvanceCursor(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	empty := runner.Measure(crossSection)

	if len(empty) != 0 {
		t.Fatalf("expected no measurements, got %d", len(empty))
	}

	if runner.lastTimestamp != 0 {
		t.Fatalf("empty measure advanced cursor to %d", runner.lastTimestamp)
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second).Add(time.Millisecond)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5},{"symbol":"ETH/USD","last":50,"volume":3,"change_pct":0.2,"bid":49.5,"ask":50.5},{"symbol":"SOL/USD","last":10,"volume":1,"change_pct":0.1,"bid":9.9,"ask":10.1}]}`))
	artifact.SetTimestamp(at.UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()

	runner.Observe(crossSection)
	runner.Measure(crossSection)

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("expected next ticker frame to replay, got %d", runner.RoleCount("ticker"))
	}
}

func TestSignalMeasureStoresMeasurements(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump: recordingSignal{},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second).Add(time.Millisecond)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
	artifact.SetTimestamp(at.UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()

	measurements := runner.Measure(crossSection)

	if len(measurements) != 1 {
		t.Fatalf("expected one measurement, got %d", len(measurements))
	}

	stored := false
	for range tree.Seek([]byte("measurement/BTC/USD")) {
		stored = true
		break
	}

	if !stored {
		t.Fatal("measurement was returned but not stored in the tree")
	}
}

func TestSignalMeasureFiltersConfiguredQuoteCurrency(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump: recordingSignal{},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second).Add(time.Millisecond)
	for index, symbol := range []string{"BTC/USD", "ETH/EUR"} {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope(symbol)
		artifact.WithPayload([]byte(fmt.Sprintf(
			`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`,
			symbol,
		)))
		artifact.SetTimestamp(at.UnixNano() + int64(index))

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	measurements := runner.Measure(crossSection)

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("expected only USD ticker frame to replay, got %d", runner.RoleCount("ticker"))
	}

	if len(measurements) != 1 {
		t.Fatalf("expected one USD measurement, got %d", len(measurements))
	}
}

func TestSignalMeasureRunsIndependentSignalsConcurrently(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump:  blockingSignal{started: started, release: release, scope: "BTC/USD"},
		logic.SourceLiquidity: blockingSignal{started: started, release: release, scope: "ETH/USD"},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second).Add(time.Millisecond)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
	artifact.SetTimestamp(at.UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()

	done := make(chan []*datura.Artifact, 1)
	go func() {
		done <- runner.Measure(crossSection)
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(250 * time.Millisecond):
			close(release)
			t.Fatal("signals did not both start before release; Measure is still serial")
		}
	}

	close(release)

	select {
	case measurements := <-done:
		if len(measurements) != 2 {
			t.Fatalf("expected two measurements, got %d", len(measurements))
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent Measure workers did not finish")
	}
}

func TestSignalMeasureDoesNotDependOnQPoolWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 1, &qpool.Config{
		SchedulingTimeout:  20 * time.Millisecond,
		JobChannelCapacity: 1,
		Scaler:             nil,
	})

	block := make(chan struct{})
	started := make(chan struct{})
	wait := pool.Schedule("block-worker", func(context.Context) (any, error) {
		close(started)
		<-block
		return nil, nil
	})

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("qpool worker did not start blocking job")
	}

	defer func() {
		close(block)
		_, _ = wait.Get(context.Background())
		pool.Close()
	}()

	crossSection := testutil.NewTestCrossSection(t)
	tree := dmt.NewTree("")
	runner := NewSignal(ctx, pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump: recordingSignal{},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second).Add(time.Millisecond)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
	artifact.SetTimestamp(at.UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()

	measurements := runner.Measure(crossSection)

	if len(measurements) != 1 {
		t.Fatalf("expected one measurement while qpool was busy, got %d", len(measurements))
	}

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("expected ticker frame to replay while qpool was busy, got %d", runner.RoleCount("ticker"))
	}
}

func TestSignalMeasureKeepsBookFloods(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)

	frames := []struct {
		symbol string
		bidQty float64
		stamp  int64
	}{
		{symbol: "BTC/USD", bidQty: 1, stamp: at.UnixNano() + 1},
		{symbol: "BTC/USD", bidQty: 2, stamp: at.UnixNano() + 2},
		{symbol: "ETH/USD", bidQty: 3, stamp: at.UnixNano() + 3},
	}

	for _, frame := range frames {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("book")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(fmt.Sprintf(
			`{"channel":"book","type":"update","data":[{"symbol":%q,"bids":[{"price":100,"qty":%g}],"asks":[{"price":101,"qty":1}]}]}`,
			frame.symbol,
			frame.bidQty,
		)))
		artifact.SetTimestamp(frame.stamp)

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	runner.Observe(crossSection)

	books := runner.cachedFramesByRole["book"]
	if len(books) != len(frames) {
		t.Fatalf("expected every book frame to replay, got %d", len(books))
	}

	seenBTC := 0
	for _, book := range books {
		if datura.Peek[string](book, "data", 0, "symbol") == "BTC/USD" {
			seenBTC++
		}
	}

	if seenBTC != 2 {
		t.Fatalf("expected both BTC/USD book frames, got %d", seenBTC)
	}
}

func TestSignalMeasureLiveTickerShapeProducesMeasurements(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	payload := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"BTC/USD","bid":60108.6,"bid_qty":2.84311954,"ask":60108.7,"ask_qty":0.00138853,"last":60108.7,"volume":4473.99704028,"vwap":59460.1,"low":58033,"high":61858.8,"change":-1497.7,"change_pct":-2.43,"timestamp":"2026-06-26T06:41:22.476797Z"}]}`)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("snapshot")
	artifact.WithPayload(payload)
	artifact.SetTimestamp(time.Now().UTC().UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()

	runner.Observe(crossSection)
	measurements := runner.Measure(crossSection)

	if len(measurements) == 0 {
		t.Fatal("expected live ticker-shaped frame to produce measurements")
	}
}

func TestSignalMeasureConcurrentMultiRole(t *testing.T) {
	Convey("Given book, trade, and ticker frames for the same symbol", t, func() {
		// Multi-role signals (manifold) are scored from one frame each of their
		// roles. Run under -race: per-signal fan-out must keep each signal
		// single-threaded so no shared field/universe state is raced.
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 4, 8, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)
		defer runner.Close()

		at := time.Now().UTC().Truncate(time.Second)

		frames := []struct {
			role    string
			payload string
		}{
			{"ticker", `{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5},{"symbol":"ETH/USD","last":50,"volume":3,"change_pct":0.2,"bid":49.5,"ask":50.5},{"symbol":"SOL/USD","last":10,"volume":1,"change_pct":0.1,"bid":9.9,"ask":10.1}]}`},
			{"book", `{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"price":99.5,"qty":2}],"asks":[{"price":100.5,"qty":3}]}]}`},
			{"trade", `{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":100,"qty":1,"side":"buy"}]}`},
		}

		for index, frame := range frames {
			artifact := datura.Acquire("kraken:public", datura.APPJSON)
			artifact.WithRole(frame.role)
			artifact.WithScope("update")
			artifact.WithPayload([]byte(frame.payload))
			artifact.SetTimestamp(at.UnixNano() + int64(index))

			tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
			artifact.Release()
		}

		runner.Observe(crossSection)
		runner.Measure(crossSection)

		Convey("It scores every signal without racing shared state", func() {
			So(runner.RoleCount("ticker"), ShouldEqual, 1)
			So(runner.RoleCount("book"), ShouldEqual, 1)
			So(runner.RoleCount("trade"), ShouldEqual, 1)
		})
	})
}

func TestSignalMeasureIncrementalSeek(t *testing.T) {
	Convey("Given two ticker ingest rows", t, func() {
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		firstAt := time.Now().UTC().Truncate(time.Second)
		secondAt := firstAt
		runner.lastTimestamp = firstAt.Add(-time.Nanosecond).UnixNano()

		for index, at := range []time.Time{firstAt, secondAt} {
			artifact := datura.Acquire("kraken:public", datura.APPJSON)
			artifact.WithRole("ticker")
			artifact.WithScope("update")
			artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5},{"symbol":"ETH/USD","last":50,"volume":3,"change_pct":0.2,"bid":49.5,"ask":50.5},{"symbol":"SOL/USD","last":10,"volume":1,"change_pct":0.1,"bid":9.9,"ask":10.1}]}`))
			artifact.SetTimestamp(at.UnixNano() + int64(index))

			tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
			artifact.Release()
		}

		// Mirror the trader: observe the peer snapshot before measuring.
		runner.Observe(crossSection)
		first := runner.Measure(crossSection)
		firstTickerCount := runner.RoleCount("ticker")
		lastObserved := runner.lastTimestamp
		second := runner.Measure(crossSection)
		secondTickerCount := runner.RoleCount("ticker")

		Convey("It should only replay unseen rows on the second pass", func() {
			So(len(first), ShouldBeGreaterThanOrEqualTo, 0)
			So(firstTickerCount, ShouldEqual, 2)
			So(len(second), ShouldEqual, 0)
			So(secondTickerCount, ShouldEqual, 0)
			So(runner.lastTimestamp, ShouldEqual, lastObserved)
			So(runner.lastTimestampByRole["ticker"], ShouldBeGreaterThan, lastObserved)
		})
	})
}

func BenchmarkSignalMeasureIncrementalSeek(b *testing.B) {
	crossSection := testutil.NewTestCrossSection(b)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	defer runner.Close()

	at := time.Now().UTC().Truncate(time.Second)

	for index := range 128 {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(at.Add(time.Duration(index) * time.Millisecond).UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	runner.Measure(crossSection)

	b.ReportAllocs()

	for b.Loop() {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(at.Add(time.Duration(b.N) * time.Millisecond).UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()

		runner.Measure(crossSection)
	}
}
