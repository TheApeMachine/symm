package trader

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/signal/testutil"
	"github.com/theapemachine/symm/statutil"
	bookfixtures "github.com/theapemachine/symm/tests/fixtures/book"
	levelfixtures "github.com/theapemachine/symm/tests/fixtures/level3"
	tickerfixtures "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixtures "github.com/theapemachine/symm/tests/fixtures/trade"
)

func init() {
	viper.SetDefault("market.book_depth_levels", 10)
}

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

type burstSignal struct{}

func (burstSignal) IngestRoles() []string {
	return []string{"ticker"}
}

func (burstSignal) Measure(*datura.Artifact, *market.CrossSection) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		for timestamp := int64(1); timestamp <= 3; timestamp++ {
			measurement := datura.Acquire("burst", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope("BTC/USD")
			measurement.WithPayload([]byte(`{"ok":true}`))
			measurement.SetTimestamp(timestamp)
			if !yield(measurement) {
				return
			}
		}
	}
}

func (burstSignal) Close() error {
	return nil
}

func TestCoalesceSnapshotFrames(t *testing.T) {
	older := datura.Acquire("kraken:public", datura.APPJSON)
	older.WithRole("ticker")
	older.WithScope("BTC/USD")
	older.SetTimestamp(100)
	defer older.Release()

	newer := datura.Acquire("kraken:public", datura.APPJSON)
	newer.WithRole("ticker")
	newer.WithScope("BTC/USD")
	newer.SetTimestamp(200)
	defer newer.Release()

	other := datura.Acquire("kraken:public", datura.APPJSON)
	other.WithRole("ticker")
	other.WithScope("ETH/USD")
	other.SetTimestamp(150)
	defer other.Release()

	tickerFrames := coalesceSnapshotFrames("ticker", []*datura.Artifact{
		older,
		newer,
		other,
	})
	if len(tickerFrames) != 2 {
		t.Fatalf("ticker frames=%d, want latest per scope", len(tickerFrames))
	}
	if tickerFrames[1] != newer {
		t.Fatalf("BTC/USD ticker frame was not the newest snapshot")
	}

	bookFrames := coalesceSnapshotFrames("book", []*datura.Artifact{
		older,
		newer,
		other,
	})
	if len(bookFrames) != 3 {
		t.Fatalf("book frames=%d, want event stream uncollapsed", len(bookFrames))
	}

	tradeFrames := coalesceSnapshotFrames("trade", []*datura.Artifact{
		older,
		newer,
		other,
	})
	if len(tradeFrames) != 3 {
		t.Fatalf("trade frames=%d, want event stream uncollapsed", len(tradeFrames))
	}
}

func TestMeasureSignalPublishesLatestMeasurementPerScope(t *testing.T) {
	tree := dmt.NewTree("")
	frame := datura.Acquire("kraken:public", datura.APPJSON)
	frame.WithRole("ticker")
	frame.WithScope("BTC/USD")
	frame.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD"}]}`))
	frame.SetTimestamp(time.Now().UTC().UnixNano())
	defer frame.Release()

	result := measureSignal(
		logic.SourceFluid,
		burstSignal{},
		tree,
		map[string][]*datura.Artifact{"ticker": {frame}},
		nil,
	)

	if len(result.measurements) != 1 {
		t.Fatalf("measurements = %d, want latest per scope only", len(result.measurements))
	}
	if result.measurements[0].Timestamp() != 3 {
		t.Fatalf("published timestamp = %d, want latest timestamp 3", result.measurements[0].Timestamp())
	}
	if role, _ := result.measurements[0].Role(); role != "measurement" {
		t.Fatalf("measurement role = %q, want measurement", role)
	}
	if origin, _ := result.measurements[0].Origin(); origin != string(logic.SourceFluid) {
		t.Fatalf("measurement origin = %q, want %s", origin, logic.SourceFluid)
	}

	stored := 0
	tree.WalkPrefix([]byte("measurement/BTC/USD/fluid/"), func(_, _ []byte) bool {
		stored++
		return true
	})
	if stored != 1 {
		t.Fatalf("stored measurements = %d, want 1", stored)
	}
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

func TestSignalMeasurePrunesProcessedTickerSnapshots(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	at := time.Now().UTC().Truncate(time.Second)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("BTC/USD")
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
	artifact.SetTimestamp(at.UnixNano())

	tree.Insert(artifact.Prefix("role", "timestamp", "scope"), artifact.Pack())
	artifact.Release()

	runner.Observe(crossSection)
	measurements := runner.Measure(crossSection)

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("ticker frame was not replayed before pruning: got %d", runner.RoleCount("ticker"))
	}

	if len(measurements) == 0 {
		t.Fatalf("expected measurements from replayed ticker frame")
	}

	for remaining := range tree.Seek([]byte("ticker/")) {
		t.Fatalf("processed ticker snapshot was not pruned: %v", remaining)
	}
}

func TestSignalMeasurePrunesCoalescedTickerBacklog(t *testing.T) {
	crossSection := testutil.NewTestCrossSection(t)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	defer runner.Close()

	base := time.Now().UTC().Add(-time.Second).Truncate(time.Second)

	for index := range 3 {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("BTC/USD")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(base.Add(time.Duration(index) * time.Millisecond).UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp", "scope"), artifact.Pack())
		artifact.Release()
	}

	runner.Observe(crossSection)
	runner.Measure(crossSection)

	if runner.RoleCount("ticker") != 1 {
		t.Fatalf("ticker frames should be coalesced for scoring: got %d", runner.RoleCount("ticker"))
	}

	for remaining := range tree.Seek([]byte("ticker/")) {
		t.Fatalf("coalesced-away ticker cursor frame was not pruned: %v", remaining)
	}
}

func TestSignalPrunesProcessedCursorFrameButKeepsScopedHistory(t *testing.T) {
	tree := dmt.NewTree("")
	runner := &Signal{tree: tree}
	at := time.Now().UTC().Truncate(time.Second)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("BTC/USD")
	artifact.WithPayload([]byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":99,"qty":1}],"asks":[{"price":100,"qty":1}]}]}`))
	artifact.SetTimestamp(at.UnixNano())
	defer artifact.Release()

	cursorKey := artifact.Prefix("role", "timestamp", "scope")
	scopedKey := artifact.Prefix("role", "scope", "timestamp")

	tree.InsertArtifact(cursorKey, artifact)
	tree.InsertArtifact(scopedKey, artifact)

	runner.pruneProcessedCursorFrames(map[string][]*datura.Artifact{
		"book": {artifact},
	})

	if _, ok := tree.Get(cursorKey); ok {
		t.Fatal("processed cursor key was not pruned")
	}

	if _, ok := tree.Get(scopedKey); !ok {
		t.Fatal("scoped book history was pruned")
	}
}

func TestPruneMeasurementHistoriesBoundsTreeHistory(t *testing.T) {
	tree := dmt.NewTree("")
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	stamps := make([]float64, 0, 64)
	latest := make([]*datura.Artifact, 0, 1)
	prefix := []byte("measurement/BTC/USD/" + string(logic.SourceToxicity) + "/")

	for index := range 64 {
		measurement := datura.Acquire("test", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		measurement.SetOrigin(string(logic.SourceToxicity))
		measurement.SetTimestamp(base.Add(time.Duration(index) * time.Second).UnixNano())
		measurement.WithPayload([]byte(`{"ok":true}`))

		tree.InsertArtifact(measurement.Prefix("role", "scope", "origin", "timestamp"), measurement)
		stamps = append(stamps, float64(measurement.Timestamp()))

		if index == 63 {
			latest = append(latest, measurement)
		} else {
			measurement.Release()
		}
	}
	defer latest[0].Release()

	pruneMeasurementHistories(tree, latest)

	count := 0
	for range tree.Seek(prefix) {
		count++
	}

	want := statutil.WindowDepth(stamps)
	if count != want {
		t.Fatalf("measurement history count=%d, want cadence window %d", count, want)
	}
}

func TestPruneScopedMarketHistoriesBoundsTreeHistory(t *testing.T) {
	for _, role := range []string{"book", "ticker"} {
		t.Run(role, func(t *testing.T) {
			tree := dmt.NewTree("")
			base := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
			symbol := "BTC/USD"
			stamps := make([]float64, 0, 64)
			latest := make([]*datura.Artifact, 0, 1)
			prefix := []byte(role + "/" + symbol + "/")
			payload := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":99,"qty":1}],"asks":[{"price":100,"qty":1}]}]}`)
			if role == "ticker" {
				payload = []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1}]}`)
			}

			for index := range 64 {
				artifact := datura.Acquire("kraken:public", datura.APPJSON)
				artifact.WithRole(role)
				artifact.WithScope(symbol)
				artifact.WithPayload(payload)
				artifact.SetTimestamp(base.Add(time.Duration(index) * time.Second).UnixNano())

				tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
				stamps = append(stamps, float64(artifact.Timestamp()))

				if index == 63 {
					latest = append(latest, artifact)
				} else {
					artifact.Release()
				}
			}
			defer latest[0].Release()

			pruneScopedMarketHistories(tree, map[string][]*datura.Artifact{
				role: latest,
			})

			count := 0
			for range tree.Seek(prefix) {
				count++
			}

			want := statutil.WindowDepth(stamps)
			if count != want {
				t.Fatalf("%s history count=%d, want cadence window %d", role, count, want)
			}
		})
	}
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

func TestSignalMeasureEmptyPassAdvancesScanCursorOnly(t *testing.T) {
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
		t.Fatalf("empty measure advanced observed cursor to %d", runner.lastTimestamp)
	}

	for _, role := range ingestRoles {
		if runner.lastObservedByRole[role] != 0 {
			t.Fatalf("empty measure advanced observed cursor for %s to %d", role, runner.lastObservedByRole[role])
		}

		if runner.lastTimestampByRole[role] <= 0 {
			t.Fatalf("empty measure left scan cursor at zero for %s", role)
		}
	}
}

func TestSignalPollIntervalUsesCeilingAfterFirstObservedFrame(t *testing.T) {
	runner := &Signal{lastTimestamp: time.Now().UTC().UnixNano()}

	if got := runner.PollInterval(); got != maxPollInterval {
		t.Fatalf("first observed frame should wait at ceiling until cadence is known: got %s want %s", got, maxPollInterval)
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

func TestSignalMeasureReplaysBookSnapshotsAndUpdates(t *testing.T) {
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
	if len(books) != 3 {
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
		measurements := runner.Measure(crossSection)

		Convey("It scores every signal without racing shared state", func() {
			So(runner.RoleCount("ticker"), ShouldEqual, 1)
			So(runner.RoleCount("book"), ShouldEqual, 1)
			So(runner.RoleCount("trade"), ShouldEqual, 1)
		})

		Convey("It publishes the integrated manifold field snapshot to the UI", func() {
			received := make(chan map[string]any, 1)

			pool.Subscribe("ui", func(artifact *datura.Artifact) error {
				role, roleErr := artifact.Role()
				if roleErr != nil || role != "manifold" {
					return nil
				}

				payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)
				if decodeErr == nil {
					received <- payload
				}

				return nil
			})

			crypto := &Crypto{
				signals:     runner,
				uiBroadcast: pool.CreateBroadcastGroup("ui"),
			}
			crypto.publishManifoldSnapshot(7, measurements)

			select {
			case payload := <-received:
				So(payload["type"], ShouldEqual, "manifold")
				So(payload["rho"], ShouldNotBeNil)
				So(payload["carriers"], ShouldNotBeNil)
				So(payload["tick"], ShouldEqual, float64(7))
			case <-time.After(500 * time.Millisecond):
				So("ui manifold snapshot", ShouldEqual, "published")
			}
		})
	})
}

func TestSignalMeasureResonanceFromCachedFrames(t *testing.T) {
	Convey("Given cached live ticker and book frames", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 4, 8, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(ctx, pool, tree)
		resonanceSignal := resonance.NewSignal(ctx, pool, tree, nil, 0.02, 1)

		So(runner, ShouldNotBeNil)
		So(resonanceSignal, ShouldNotBeNil)

		defer runner.Close()
		defer func() {
			_ = resonanceSignal.Close()
		}()

		received := make(chan map[string]any, 1)
		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			role, roleErr := artifact.Role()
			if roleErr != nil || role != "resonance" {
				return nil
			}

			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)
			if decodeErr == nil && payload["type"] == "resonance_universe" {
				received <- payload
			}

			return nil
		})

		at := time.Now().UTC().Truncate(time.Second)
		frames := []struct {
			role    string
			payload string
		}{
			{"ticker", `{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1000,"change_pct":0.5,"bid":99.5,"ask":100.5,"timestamp":"2026-06-26T06:41:22.476797Z"},{"symbol":"ETH/USD","last":50,"volume":900,"change_pct":0.2,"bid":49.5,"ask":50.5,"timestamp":"2026-06-26T06:41:22.476797Z"}]}`},
			{"book", `{"channel":"book","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"price":99.5,"qty":2}],"asks":[{"price":100.5,"qty":3}]},{"symbol":"ETH/USD","bids":[{"price":49.5,"qty":1}],"asks":[{"price":50.5,"qty":2}]}]}`},
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

		measurements := runner.MeasureResonance(resonanceSignal)

		Convey("It settles resonance, stores measurements, and publishes the universe snapshot", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)

			for _, measurement := range measurements {
				scope, scopeErr := measurement.Scope()

				So(scopeErr, ShouldBeNil)
				So(scope, ShouldNotEqual, "")

				found := false
				for range tree.Seek([]byte("measurement/" + scope + "/resonance/")) {
					found = true
					break
				}

				So(found, ShouldBeTrue)
			}

			select {
			case payload := <-received:
				So(payload["type"], ShouldEqual, "resonance_universe")
				So(payload["snapshots"], ShouldNotBeNil)
			case <-time.After(500 * time.Millisecond):
				So("ui resonance universe snapshot", ShouldEqual, "published")
			}
		})
	})
}

func TestSignalMeasureKrakenFixtureStream(t *testing.T) {
	Convey("Given a short Kraken-shaped fixture stream", t, func() {
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 4, 8, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
		insertFixtureArtifacts(tree, at, tickerfixtures.NewFixture(tickerfixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(10*time.Second), bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts())
		insertFixtureArtifacts(tree, at.Add(20*time.Second), bookfixtures.NewFixture(bookfixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(30*time.Second), tradefixtures.NewFixture(tradefixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(40*time.Second), levelfixtures.NewFixture(levelfixtures.UPDATE, 3).Artifacts())

		Convey("When the signal runner observes and measures the stream", func() {
			So(func() {
				runner.Observe(crossSection)
				runner.Measure(crossSection)
			}, ShouldNotPanic)

			Convey("Then it should keep the latest ticker and every event-stream frame", func() {
				So(runner.RoleCount("ticker"), ShouldEqual, 1)
				So(runner.RoleCount("book"), ShouldEqual, 4)
				So(runner.RoleCount("trade"), ShouldEqual, 3)
				So(runner.RoleCount("level3"), ShouldEqual, 3)
			})
		})
	})
}

func insertFixtureArtifacts(
	tree *dmt.Tree,
	start time.Time,
	artifacts iter.Seq[*datura.Artifact],
) {
	index := 0

	for artifact := range artifacts {
		if symbol := datura.Peek[string](artifact, "data", 0, "symbol"); symbol != "" {
			artifact.WithScope(symbol)
		}

		artifact.SetTimestamp(start.Add(time.Duration(index) * time.Second).UnixNano())
		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
		index++
	}
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
			So(firstTickerCount, ShouldEqual, 1)
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

func BenchmarkSignalMeasureQuoteBoard(b *testing.B) {
	crossSection := testutil.NewTestCrossSection(b)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	defer runner.Close()

	at := time.Now().UTC().Truncate(time.Second)

	b.ReportAllocs()

	for b.Loop() {
		insertScopedTickerBoard(tree, at.Add(time.Duration(b.N)*time.Millisecond), 397)
		runner.Observe(crossSection)
		runner.Measure(crossSection)
	}
}

func insertScopedTickerBoard(tree *dmt.Tree, at time.Time, symbols int) {
	for index := range symbols {
		symbol := fmt.Sprintf("SYM%03d/USD", index)
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope(symbol)
		artifact.WithPayload(scopedTickerPayload(symbol, index))
		artifact.SetTimestamp(at.UnixNano() + int64(index))

		tree.Insert(artifact.Prefix("role", "timestamp", "scope"), artifact.Pack())
		artifact.Release()
	}
}

func scopedTickerPayload(symbol string, index int) []byte {
	var payload strings.Builder

	fmt.Fprintf(
		&payload,
		`{"channel":"ticker","type":"update","data":[{"symbol":"%s","last":%.4f,"volume":%.4f,"change_pct":%.4f,"bid":%.4f,"ask":%.4f}]}`,
		symbol,
		100+float64(index)*0.01,
		1+float64(index%17),
		float64(index%9)*0.05,
		99.5+float64(index)*0.01,
		100.5+float64(index)*0.01,
	)

	return []byte(payload.String())
}
