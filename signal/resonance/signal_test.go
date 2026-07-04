package resonance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func resonanceTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func insertFeedArtifact(signal *Signal, role, scope string, payload any) {
	raw, err := json.Marshal(payload)

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(raw)

	signal.ObserveIngest(artifact)

	if wire := artifact.Pack(); len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func settledMeasurement(signal *Signal, scope string) *datura.Artifact {
	results, err := signal.SettleScopes([]string{scope})

	if err != nil {
		return nil
	}

	return results[scope]
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func storeResonanceMeasurement(signal *Signal, measurement *datura.Artifact) {
	if measurement != nil {
		updated, _, _ := signal.tree.InsertArtifact(measurement.Prefix(), measurement)

		if updated != nil {
			signal.tree = updated
		}
	}
}

func seedMarketFixture(
	signal *Signal,
	scope string,
	last float64,
	volume float64,
	changePct float64,
	spreadRatio float64,
	observedAt time.Time,
) {
	bid := last * (1 - spreadRatio/2)
	ask := last * (1 + spreadRatio/2)

	insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
		Symbol:    scope,
		Last:      last,
		Volume:    volume,
		ChangePct: changePct,
		Timestamp: observedAt,
	}})
	insertFeedArtifact(signal, "book", scope, []bookFixture{{
		Symbol: scope,
		Bids:   []bookLevelFixture{{Price: bid, Qty: 1}},
		Asks:   []bookLevelFixture{{Price: ask, Qty: 1}},
	}})
}

func TestSignalMeasure(testingTB *testing.T) {
	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given laminar market hydration fixtures", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "FLOW/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 0.001, observedAt)

		result := settledMeasurement(signal, scope)
		storeResonanceMeasurement(signal, result)

		Convey("It should classify laminar resonance and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "value"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "entry_baseline"), ShouldAlmostEqual, 1.0/float64(resonanceLatentWidth), 1e-12)
			So(datura.Peek[float64](result, "output", "exit_baseline"), ShouldAlmostEqual, 1.0/float64(resonanceLatentWidth), 1e-12)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)

			origin, originErr := result.Origin()

			So(originErr, ShouldBeNil)
			So(origin, ShouldEqual, "resonance")

			result.Release()
		})
	})

	Convey("Given a wider-spread laminar hydration fixture", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "FLOW/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 0.002, observedAt)

		result := settledMeasurement(signal, scope)
		storeResonanceMeasurement(signal, result)

		Convey("It should still classify laminar resonance and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given equilibrium market hydration fixtures", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "COUPLE/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 2.001, observedAt)

		result := settledMeasurement(signal, scope)
		storeResonanceMeasurement(signal, result)

		Convey("It should classify equilibrium coupling and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := settledMeasurement(signal, "NEW/EUR")

		Convey("It should return nil without publishing", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given laminar market hydration fixtures", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "FLOW/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 0.001, observedAt)

		result := settledMeasurement(signal, scope)
		storeResonanceMeasurement(signal, result)

		Convey("It should classify laminar resonance and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given equilibrium market hydration fixtures", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), dmt.NewTree(""), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "COUPLE/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 2.001, observedAt)

		result := settledMeasurement(signal, scope)
		storeResonanceMeasurement(signal, result)

		Convey("It should classify equilibrium coupling and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	viper.Set("signals.feed_ring_capacity", 64)

	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), resonanceTestPool(b), dmt.NewTree(""), nil, 0.02, 8)

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		seedMarketFixture(signal, "FLOW/EUR", 1, 1, -2, 0.001, observedAt)
		result := settledMeasurement(signal, "FLOW/EUR")
		storeResonanceMeasurement(signal, result)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "FLOW/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/FLOW/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
