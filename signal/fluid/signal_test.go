package fluid

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
)

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) BookUpdate {
	bids := []BookLevel{
		{Price: bidPrice, Qty: bidQty},
		{Price: bidPrice - 0.01, Qty: bidQty * 0.5},
	}
	asks := []BookLevel{
		{Price: askPrice, Qty: askQty},
		{Price: askPrice + 0.01, Qty: askQty * 0.5},
	}

	return BookUpdate{
		Symbol: fixture.symbol,
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	}
}

func seedFluidConfig() {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("measurement", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](testingTB.Context(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertFluidFeatures(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given laminar fluid features", testingTB, func() {
		seedFluidConfig()
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		insertFluidFeatures(signal, "LAM/EUR", laminarFluidPayload()...)

		result := signal.Measure(measurementQuery("LAM/EUR"))

		Convey("It should classify laminar flow and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "LAM/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "LAM/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given turbulent fluid features", testingTB, func() {
		seedFluidConfig()
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		insertFluidFeatures(signal, "TURB/EUR", turbulentFluidPayload()...)

		result := signal.Measure(measurementQuery("TURB/EUR"))

		Convey("It should classify turbulent flow and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "TURB/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "TURB/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given inertial fluid features", testingTB, func() {
		seedFluidConfig()
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		insertFluidFeatures(signal, "INER/EUR", inertialFluidPayload()...)

		result := signal.Measure(measurementQuery("INER/EUR"))

		Convey("It should classify inertial displacement and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "INER/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "INER/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given viscous fluid features", testingTB, func() {
		seedFluidConfig()
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		insertFluidFeatures(signal, "VISC/EUR", viscousFluidPayload()...)

		result := signal.Measure(measurementQuery("VISC/EUR"))

		Convey("It should classify viscous resistance and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "VISC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "VISC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func insertTreeIngest(signal *Signal, role, scope string, payload any) {
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

func TestSignalMeasureBookAfterFeed(testingTB *testing.T) {
	Convey("Given warmed book and ticker rows in the tree", testingTB, func() {
		seedFluidConfig()
		symbol := "BTC/EUR"
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		fixture := symbolBookFixture{symbol: symbol}

		defer func() {
			_ = signal.Close()
		}()

		insertTreeIngest(signal, "ticker", symbol, []TickerUpdate{{
			Symbol: symbol, Last: 100, Bid: 99.99, Ask: 100.01, Volume: 1000, Timestamp: feedAt,
		}})

		firstBook := fixture.snapshot(99.99, 5, 100.01, 5)
		firstBook.Timestamp = feedAt
		insertTreeIngest(signal, "book", symbol, []BookUpdate{firstBook})

		replenish := fixture.snapshot(99.99, 8, 100.01, 8)
		replenish.Timestamp = feedAt.Add(100 * time.Millisecond)
		insertTreeIngest(signal, "book", symbol, []BookUpdate{replenish})

		result := signal.Measure(measurementQuery(symbol))

		Convey("It should publish a classified measurement without Update relay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, symbol)
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, symbol), ShouldBeTrue)
			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(testingTB *testing.B) {
	seedFluidConfig()
	query := measurementQuery("LAM/EUR")
	payload := laminarFluidPayload()

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())

		if signal == nil {
			testingTB.Fatal("NewSignal returned nil")
		}

		insertFluidFeatures(signal, "LAM/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			testingTB.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "LAM/EUR") {
			testingTB.Fatal("InsertMeasurement did not index measurement/LAM/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
