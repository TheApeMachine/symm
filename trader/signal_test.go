package trader

import (
	"context"
	"fmt"
	"iter"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	bookfixtures "github.com/theapemachine/symm/tests/fixtures/book"
	levelfixtures "github.com/theapemachine/symm/tests/fixtures/level3"
	tickerfixtures "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixtures "github.com/theapemachine/symm/tests/fixtures/trade"
)

func init() {
	viper.SetDefault("market.book_depth_levels", 10)
}

type recordingSignal struct {
	roles []string
}

func (signal recordingSignal) IngestRoles() []string {
	return signal.roles
}

func (recordingSignal) Measure(
	artifact *datura.Artifact,
	_ *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		scope, _ := artifact.Scope()
		measurement := datura.Acquire("recording", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope(scope)
		measurement.WithPayload([]byte(`{"ok":true}`))
		measurement.SetTimestamp(artifact.Timestamp())
		yield(measurement)
	}
}

func (recordingSignal) Close() error {
	return nil
}

func TestSignalMeasureReplaysRoleScopeSecondArtifacts(t *testing.T) {
	Convey("Given ticker artifacts keyed by role, scope, and second", t, func() {
		runner, tree := newSignalRunner(t)
		defer runner.Close()

		runner.signals = map[logic.SourceType]market.Signal{
			logic.SourcePumpDump: recordingSignal{roles: []string{"ticker"}},
		}

		at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
		runner.lastTimestamp = at.Add(-time.Second).Unix()

		insertTicker(tree, "BTC/USD", at.UnixNano())
		insertTicker(tree, "ETH/USD", at.Add(time.Millisecond).UnixNano())

		measurements := runner.Measure()

		Convey("It should replay every artifact in that second exactly once", func() {
			So(len(measurements), ShouldEqual, 2)
			So(scopes(measurements), ShouldResemble, []string{"update", "update"})
		})
	})
}

func TestSignalMeasureDoesNotReplayBeforeCursor(t *testing.T) {
	runner, tree := newSignalRunner(t)
	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump: recordingSignal{roles: []string{"ticker"}},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	runner.lastTimestamp = at.Unix()
	insertTicker(tree, "BTC/USD", at.UnixNano())

	measurements := runner.Measure()

	if len(measurements) != 0 {
		t.Fatalf("expected no measurements before cursor, got %d", len(measurements))
	}
}

func TestSignalMeasureRoutesDeclaredRoles(t *testing.T) {
	runner, tree := newSignalRunner(t)
	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourceDepthFlow: recordingSignal{roles: []string{"book", "trade"}},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	runner.lastTimestamp = at.Add(-time.Second).Unix()

	insertFrame(tree, "ticker", "update", at.UnixNano(), tickerPayload("BTC/USD"))
	insertFrame(tree, "book", "update", at.Add(time.Millisecond).UnixNano(), `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":2}],"asks":[{"price":101,"qty":1}]}]}`)
	insertFrame(tree, "trade", "update", at.Add(2*time.Millisecond).UnixNano(), `{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":100,"qty":1,"side":"buy"}]}`)

	measurements := runner.Measure()

	if len(measurements) != 2 {
		t.Fatalf("expected book and trade measurements only, got %d", len(measurements))
	}
}

func TestSignalMeasureLiveTickerShapeProducesMeasurements(t *testing.T) {
	runner, tree := newSignalRunner(t)
	defer runner.Close()

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	runner.lastTimestamp = at.Add(-time.Second).Unix()

	insertFrame(tree, "book", "update", at.UnixNano(), `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":60108.6,"qty":2.84311954}],"asks":[{"price":60108.7,"qty":0.00138853}]}]}`)
	insertFrame(tree, "trade", "update", at.Add(time.Millisecond).UnixNano(), `{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":60108.7,"qty":25,"side":"buy"}]}`)
	insertFrame(
		tree,
		"ticker",
		"update",
		at.Add(time.Second).UnixNano(),
		`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","bid":60108.6,"bid_qty":2.84311954,"ask":60108.7,"ask_qty":0.00138853,"last":60108.7,"volume":4473.99704028,"vwap":59460.1,"low":58033,"high":61858.8,"change":100,"change_pct":0.17,"timestamp":"2026-06-26T06:41:22.476797Z"}]}`,
	)

	measurements := runner.Measure()

	if len(measurements) == 0 {
		t.Fatal("expected live ticker-shaped frame to produce measurements")
	}
}

func TestSignalMeasureKrakenFixtureStream(t *testing.T) {
	Convey("Given a short Kraken-shaped fixture stream", t, func() {
		runner, tree := newSignalRunner(t)
		defer runner.Close()

		runner.signals = map[logic.SourceType]market.Signal{
			logic.SourcePumpDump: recordingSignal{roles: []string{"ticker", "book", "trade", "level3"}},
		}

		at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
		runner.lastTimestamp = at.Add(-time.Second).Unix()

		insertFixtureArtifacts(tree, at, tickerfixtures.NewFixture(tickerfixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(10*time.Second), bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts())
		insertFixtureArtifacts(tree, at.Add(20*time.Second), bookfixtures.NewFixture(bookfixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(30*time.Second), tradefixtures.NewFixture(tradefixtures.UPDATE, 3).Artifacts())
		insertFixtureArtifacts(tree, at.Add(40*time.Second), levelfixtures.NewFixture(levelfixtures.UPDATE, 3).Artifacts())

		measurements := runner.Measure()

		Convey("It should replay every declared-role artifact", func() {
			So(len(measurements), ShouldEqual, 12)
		})
	})
}

func TestSignalMeasureKrakenFixturesProduceRealSignalMeasurements(t *testing.T) {
	runner, tree := newSignalRunner(t)
	defer runner.Close()

	at := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Second)
	runner.lastTimestamp = at.Add(-time.Second).Unix()

	insertFixtureArtifacts(tree, at, tickerfixtures.NewFixture(tickerfixtures.UPDATE, 8).Artifacts())
	insertPeerTickerCohort(tree, at.Add(2*time.Second), 8)
	insertFixtureArtifacts(tree, at.Add(30*time.Second), bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts())
	insertFixtureArtifacts(tree, at.Add(100*time.Second), bookfixtures.NewFixture(bookfixtures.UPDATE, 8).Artifacts())
	insertFixtureArtifacts(tree, at.Add(210*time.Second), tradefixtures.NewFixture(tradefixtures.UPDATE, 8).Artifacts())
	insertFixtureArtifacts(tree, at.Add(230*time.Second), levelfixtures.NewFixture(levelfixtures.UPDATE, 8).Artifacts())
	insertFrame(tree, "trade", "update", at.Add(238*time.Second).UnixNano(), `{"channel":"trade","type":"update","data":[{"symbol":"ALGO/USD","price":0.101,"qty":500,"side":"buy"}]}`)
	insertFrame(tree, "ticker", "update", at.Add(241*time.Second).UnixNano(), `{"channel":"ticker","type":"update","data":[{"symbol":"ALGO/USD","bid":0.199,"bid_qty":500,"ask":0.201,"ask_qty":500,"last":0.2,"volume":997800,"change":0.099,"change_pct":98.0}]}`)

	measurements := runner.Measure()
	if len(measurements) == 0 {
		t.Fatal("expected real signals to produce fixture-backed measurements")
	}

	seen := make(map[logic.SourceType]int)
	for _, measurement := range measurements {
		if role := datura.Peek[string](measurement, "role"); role != "measurement" {
			t.Fatalf("measurement role = %q, want measurement", role)
		}

		scope, scopeErr := measurement.Scope()
		if scopeErr != nil || scope == "" {
			t.Fatalf("measurement missing scope: %v", scopeErr)
		}

		origin, originErr := measurement.Origin()
		if originErr != nil || origin == "" {
			t.Fatalf("measurement missing origin: %v", originErr)
		}
		seen[logic.SourceType(origin)]++

		value := datura.Peek[float64](measurement, "output", "value")
		if value == 0 {
			value = datura.Peek[float64](measurement, "output", "category")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("%s/%s published non-finite category value %v", origin, scope, value)
		}
		if _, ok := logic.Categories[int(value)]; !ok {
			t.Fatalf("%s/%s published unknown category index %v", origin, scope, value)
		}

		for _, field := range []string{"confidence", "strength"} {
			score := datura.Peek[float64](measurement, "output", field)
			if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 {
				t.Fatalf("%s/%s output.%s = %v", origin, scope, field, score)
			}
		}
	}

	required := []logic.SourceType{
		logic.SourcePumpDump,
		logic.SourceLiquidity,
		logic.SourceSentiment,
		logic.SourceCorrelation,
		logic.SourceLeadLag,
		logic.SourceHawkes,
		logic.SourceCVD,
		logic.SourceDepthFlow,
		logic.SourceExhaustion,
		logic.SourceCausal,
		logic.SourceToxicity,
	}
	missing := make([]logic.SourceType, 0)
	for _, source := range required {
		if seen[source] == 0 {
			missing = append(missing, source)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing fixture-backed measurements from %v; seen %v", missing, seen)
	}
}

func BenchmarkSignalMeasureSecondPrefix(b *testing.B) {
	runner, tree := newSignalRunner(b)
	defer runner.Close()

	runner.signals = map[logic.SourceType]market.Signal{
		logic.SourcePumpDump: recordingSignal{roles: []string{"ticker"}},
	}

	at := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	runner.lastTimestamp = at.Add(-time.Second).Unix()

	for index := range 128 {
		insertTicker(tree, fmt.Sprintf("SYM-%d/USD", index), at.Add(time.Duration(index)*time.Millisecond).UnixNano())
	}

	b.ReportAllocs()

	for b.Loop() {
		runner.lastTimestamp = at.Add(-time.Second).Unix()
		_ = runner.Measure()
	}
}

func newSignalRunner(t testing.TB) (*Signal, *dmt.Tree) {
	t.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	if runner == nil {
		t.Fatal("expected signal runner")
	}

	return runner, tree
}

func insertTicker(tree *dmt.Tree, symbol string, stamp int64) {
	insertFrame(tree, "ticker", "update", stamp, tickerPayload(symbol))
}

func tickerPayload(symbol string) string {
	return fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":100,"volume":5,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`,
		symbol,
	)
}

func insertPeerTickerCohort(tree *dmt.Tree, start time.Time, samples int) {
	peers := []struct {
		symbol string
		price  float64
		step   float64
		change float64
		volume float64
	}{
		{symbol: "BTC/USD", price: 60000, step: 90, change: 4.0, volume: 1200},
		{symbol: "ETH/USD", price: 3200, step: 7, change: 1.4, volume: 900},
		{symbol: "SOL/USD", price: 140, step: 0.6, change: 1.1, volume: 700},
		{symbol: "MATIC/USD", price: 0.50, step: 0.004, change: 2.2, volume: 500},
	}

	for sample := range samples {
		for peerIndex, peer := range peers {
			price := peer.price + peer.step*float64(sample)
			stamp := start.Add(
				time.Duration(sample)*time.Second +
					time.Duration(peerIndex)*time.Millisecond,
			).UnixNano()

			insertFrame(
				tree,
				"ticker",
				"update",
				stamp,
				fmt.Sprintf(
					`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%f,"volume":%f,"change_pct":%f,"bid":%f,"ask":%f}]}`,
					peer.symbol,
					price,
					peer.volume+float64(sample)*10,
					peer.change+float64(sample)*0.05,
					price*0.999,
					price*1.001,
				),
			)
		}
	}
}

func insertFrame(tree *dmt.Tree, role string, scope string, stamp int64, payload string) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(stamp)
	insertTreeArtifact(tree, artifact)
	artifact.Release()
}

func insertTreeArtifact(tree *dmt.Tree, artifact *datura.Artifact) {
	if tree == nil || artifact == nil {
		return
	}

	tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
}

func insertFixtureArtifacts(
	tree *dmt.Tree,
	start time.Time,
	artifacts iter.Seq[*datura.Artifact],
) {
	index := 0

	for artifact := range artifacts {
		artifact.SetTimestamp(start.Add(time.Duration(index) * time.Second).UnixNano())
		insertTreeArtifact(tree, artifact)
		artifact.Release()
		index++
	}
}

func scopes(artifacts []*datura.Artifact) []string {
	values := make([]string, 0, len(artifacts))

	for _, artifact := range artifacts {
		scope, _ := artifact.Scope()
		values = append(values, scope)
	}

	return values
}
