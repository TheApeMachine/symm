package fluid

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) krakenmarket.BookUpdate {
	bids := []krakenmarket.BookLevel{
		{Price: bidPrice, Qty: bidQty},
		{Price: bidPrice - 0.01, Qty: bidQty * 0.5},
	}
	asks := []krakenmarket.BookLevel{
		{Price: askPrice, Qty: askQty},
		{Price: askPrice + 0.01, Qty: askQty * 0.5},
	}

	return krakenmarket.BookUpdate{
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

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("measurement", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
}

func newTestSignal(testingTB testing.TB) *Signal {
	testingTB.Helper()

	pool := qpool.NewQ[any](testingTB.Context(), 1, 2, nil)
	signal := NewSignal(testingTB.Context(), pool)
	testingTB.Cleanup(func() {
		_ = signal.Close()
	})

	return signal
}

func TestFluidSymbolMeasureLaminarField(testingTB *testing.T) {
	Convey("Given a balanced book with no Reynolds activity", testingTB, func() {
		seedFluidConfig()
		symbol := "BTC/EUR"
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99.99, Ask: 100.01, Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), feedAt), ShouldBeNil)

		replenish := fixture.snapshot(99.99, 8, 100.01, 8)
		So(state.FeedBook(replenish, feedAt.Add(100*time.Millisecond)), ShouldBeNil)

		reading, ok := state.Reading()
		So(ok, ShouldBeTrue)

		signal := newTestSignal(testingTB)
		insertFluidFeatures(signal, symbol, fluidReadingSamples(state, reading)...)
		result := signal.Measure(measurementQuery(symbol))

		Convey("It should publish an eligible flow reading", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(fluidCategory(datura.Peek[int](result, "classifier.category")), ShouldNotEqual, logic.CategoryTypeNone)
		})
	})
}

func fluidReadingSamples(state *FluidSymbol, reading fluidReading) []float64 {
	turbulentFloor, turbulentReady := reading.dynamics.turbulentReynoldsFloor()
	icebergScore := reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)
	turbulentReadyFlag := 0.0

	if turbulentReady {
		turbulentReadyFlag = 1
	}

	changePct := state.changePct

	if changePct <= 0 && reading.spreadBPS > 0 {
		changePct = reading.spreadBPS / 10000
	}

	return []float64{
		reading.reynolds,
		math.Abs(reading.divergence),
		reading.viscosity,
		reading.midAddRate,
		reading.midExecuteRate,
		reading.dynamics.laminarReynoldsCeiling(reading.reynolds),
		turbulentFloor,
		turbulentReadyFlag,
		reading.dynamics.laminarDivergenceEdge(),
		icebergScore,
		reading.price,
		reading.spreadBPS,
		changePct,
		state.volume,
	}
}

func TestSignalMeasureBookAfterFeed(testingTB *testing.T) {
	Convey("Given a warmed fluid symbol in the registry", testingTB, func() {
		seedFluidConfig()
		symbol := "ETH/EUR"
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		signal := newTestSignal(testingTB)
		fixture := symbolBookFixture{symbol: symbol}
		state := signal.registry.loadSymbol(symbol)

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)
		So(state.FeedBook(
			fixture.snapshot(99, 10, 101, 6),
			feedAt.Add(100*time.Millisecond),
		), ShouldBeNil)

		signal.publishFeatures(symbol)
		result := signal.Measure(measurementQuery(symbol))

		Convey("It should measure without error once the grid is ready", func() {
			_ = result
		})
	})
}

func TestSignalMeasureSparseTree(testingTB *testing.T) {
	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := newTestSignal(testingTB)
		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestSignalMeasureFromFeatures(testingTB *testing.T) {
	Convey("Given a laminar feature vector in the tree", testingTB, func() {
		seedFluidConfig()
		signal := newTestSignal(testingTB)

		insertFluidFeatures(signal, "BTC/EUR",
			0.5, 0.01, 0.8, 1, 1,
			2, 4, 0, 0.05, 0,
			100, 2, 0.01, 1000,
		)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify through the fluid pipeline", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func insertFluidFeatures(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
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

func BenchmarkSignalMeasure(testingTB *testing.B) {
	signal := NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 2, 4, nil))
	insertFluidFeatures(signal, "ETH/EUR",
		0.5, 0.01, 0.8, 1, 1,
		2, 4, 0, 0.05, 0,
		100, 2, 0.01, 1000,
	)
	query := measurementQuery("ETH/EUR")

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		result := signal.Measure(query)

		if result == nil {
			testingTB.Fatal("Measure returned nil")
		}
	}
}
