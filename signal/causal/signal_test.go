package causal

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

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
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

func insertTreeArtifact(signal *Signal, role, scope string, payload []byte) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func feedTrade(
	signal *Signal,
	symbol, side string,
	price, qty float64,
	at time.Time,
) {
	raw, err := json.Marshal([]tradeUpdate{{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       qty,
		Timestamp: at,
	}})

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "trade", symbol, raw)
}

func TestSignalMeasure(testingTB *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given low-energy flat trades", testingTB, func() {
		scope := "FLAT/USD"
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		seedFlatTrades(signal, scope, baseTime)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify equilibrium and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given liquidity shock trades", testingTB, func() {
		scope := "SHOCK/USD"
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		seedLiquidityShockTrades(signal, scope, baseTime)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify contagion and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given lightly mixed flow trades", testingTB, func() {
		scope := "MIX/USD"
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		seedDefaultTrades(signal, scope, baseTime)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify association and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a monotonic buy ramp", testingTB, func() {
		scope := "RAMP/USD"
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		seedRampTrades(signal, scope, baseTime)

		result := signal.Measure(measurementQuery(scope))

		Convey("It should classify intervention and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, scope)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, scope), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		scope := "NEW/USD"
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery(scope))

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, scope), ShouldBeFalse)
		})
	})
}

func TestHydrateNodeStoreFromTreeResetsFresh(testingTB *testing.T) {
	Convey("Given trades indexed in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedDefaultTrades(signal, "BTC/USD", baseTime)

		signal.hydrateNodeStoreFromTree()
		nodes := signal.nodeStore.Nodes("BTC/USD")
		firstLength := nodes.AlignedLength()

		signal.hydrateNodeStoreFromTree()
		secondLength := signal.nodeStore.Nodes("BTC/USD").AlignedLength()

		Convey("It should rebuild without duplicating ladder history", func() {
			So(firstLength, ShouldBeGreaterThanOrEqualTo, causalMinHistory)
			So(secondLength, ShouldEqual, firstLength)
		})
	})
}

func BenchmarkSignalMeasure(testingTB *testing.B) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scope := "FLAT/USD"
	query := measurementQuery(scope)

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		if signal == nil {
			testingTB.Fatal("NewSignal returned nil")
		}

		seedFlatTrades(signal, scope, baseTime)
		result := signal.Measure(query)

		if result == nil {
			testingTB.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, scope) {
			testingTB.Fatal("InsertMeasurement did not index measurement/" + scope)
		}

		result.Release()
		_ = signal.Close()
	}
}
