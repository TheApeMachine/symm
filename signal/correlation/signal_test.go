package correlation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
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

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func insertTrades(signal *Signal, scope string, updates []tradeUpdate) {
	raw, err := json.Marshal(updates)

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "trade", scope, raw)
}

func insertTradeRow(
	signal *Signal,
	symbol string,
	price, qty float64,
	eventAt time.Time,
) {
	insertTrades(signal, symbol, []tradeUpdate{{
		Symbol:    symbol,
		Price:     price,
		Qty:       qty,
		Timestamp: eventAt,
	}})
}

func insertPriceShocks(
	signal *Signal,
	symbols []string,
	prices map[string]float64,
	shocks []float64,
	eventAt time.Time,
) {
	for _, shock := range shocks {
		for _, symbol := range symbols {
			prices[symbol] *= shock
			insertTradeRow(signal, symbol, prices[symbol], 1, eventAt)
		}
	}
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	updates := make([]tradeUpdate, count)

	for index := range count {
		updates[index] = tradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	insertTrades(signal, symbol, updates)
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a correlated cross-section", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		seedHerdScenario(signal, eventAt)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify systemic herd and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a decoupled mover", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		seedAlphaScenario(signal, eventAt)

		result := signal.Measure(measurementQuery("ALT/EUR"))

		Convey("It should classify decoupled alpha and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ALT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ALT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given low-energy flat returns", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		seedNoiseScenario(signal, eventAt)

		result := signal.Measure(measurementQuery("FLAT/EUR"))

		Convey("It should classify stochastic noise and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "FLAT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "FLAT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a symbol falling against a rising cohort", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		seedStressScenario(signal, eventAt)

		result := signal.Measure(measurementQuery("STRESS/EUR"))

		Convey("It should classify divergent stress and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "STRESS/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "STRESS/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given insufficient warmup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		signal.crossSectionCfg.MinBars = 8

		defer func() {
			_ = signal.Close()
		}()

		insertTradeRow(signal, "WARMUP/EUR", 100, 1, eventAt)
		seedTrades(signal, "WARMUP/EUR", eventAt, 1, 100)

		result := signal.Measure(measurementQuery("WARMUP/EUR"))

		Convey("It should return nil before warmup completes", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "WARMUP/EUR"), ShouldBeFalse)
		})
	})

	Convey("Given a book-triggered ticker", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		insertPriceShocks(
			signal,
			[]string{"BTC/EUR", "ETH/EUR"},
			map[string]float64{"BTC/EUR": 100, "ETH/EUR": 50},
			[]float64{1.005, 1.01, 1.015, 1.02, 1.025},
			eventAt,
		)

		raw, marshalErr := json.Marshal([]tickerUpdate{{
			Symbol:    "BTC/EUR",
			Bid:       99.9,
			Ask:       100.1,
			AskQty:    1,
			BidQty:    1,
			Last:      100,
			Volume:    1000,
			Timestamp: eventAt,
		}})

		So(marshalErr, ShouldBeNil)

		insertTreeArtifact(signal, "ticker", "BTC/EUR", raw)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should publish without ticker value errors", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		seedHerdScenario(signal, eventAt)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "BTC/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/BTC/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
