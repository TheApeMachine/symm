package resonance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
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

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
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

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), NewTestTree(), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "FLOW/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 0.001, observedAt)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify laminar resonance and publish to the tree", func() {
			So(result, ShouldNotBeNil)

			resultScope, scopeErr := result.Scope()

			So(scopeErr, ShouldBeNil)
			So(resultScope, ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier", "category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)

			measurement := datura.As[logic.Measurement](result)

			if measurement.Source == "" {
				origin, _ := result.Origin()
				measurement.Source = logic.SourceType(origin)
			}

			if measurement.Symbol == "" {
				measurement.Symbol, _ = result.Scope()
			}

			So(measurement.Source, ShouldEqual, logic.SourceType("resonance"))
			So(measurement.Symbol, ShouldEqual, scope)

			result.Release()
		})
	})

	Convey("Given turbulent market hydration fixtures", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), NewTestTree(), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "FLOW/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 0.002, observedAt)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify turbulent resonance and publish to the tree", func() {
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

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), NewTestTree(), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "COUPLE/EUR"
		seedMarketFixture(signal, scope, 1, 1, -2, 2.001, observedAt)

		result := signal.Measure(measurementQuery(scope))

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

		signal := NewSignal(context.Background(), resonanceTestPool(testingTB), NewTestTree(), nil, 0.02, 8)
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without publishing", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	viper.Set("signals.feed_ring_capacity", 64)

	query := measurementQuery("FLOW/EUR")
	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), resonanceTestPool(b), NewTestTree(), nil, 0.02, 8)

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		seedMarketFixture(signal, "FLOW/EUR", 1, 1, -2, 0.001, observedAt)
		result := signal.Measure(query)

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
