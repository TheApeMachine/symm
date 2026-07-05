package causal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var causalCategories = []logic.CategoryType{
	logic.CategoryEndogenousAlpha,
	logic.CategorySystemicBeta,
	logic.CategoryLiquidityShock,
	logic.CategoryCausalNoise,
}

func TestSignalIngestRoles(testingTB *testing.T) {
	Convey("Given a causal signal", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		Convey("It declares ticker, book, and trade ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"ticker", "book", "trade"})
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given typed ticker, book, and trade rows", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		symbol := "BTC/USD"
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		count := 0

		for index := range 8 {
			at := base.Add(time.Duration(index) * time.Second)
			price := 100 + float64(index)

			measurements, err := signal.Measure(
				tickerInput(symbol, price, price-0.01, price+0.01, 10, 10, 0.1, at),
				nil,
			)
			So(err, ShouldBeNil)
			count += assertMeasurements(measurements, symbol)

			measurements, err = signal.Measure(
				bookInput(symbol, price-0.01, price+0.01, 10, 10, at),
				nil,
			)
			So(err, ShouldBeNil)
			count += assertMeasurements(measurements, symbol)

			measurements, err = signal.Measure(
				tradeInput(symbol, "buy", price, 1+float64(index), at),
				nil,
			)
			So(err, ShouldBeNil)
			count += assertMeasurements(measurements, symbol)
		}

		Convey("It routes rows into causal measurements", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a partial ticker update without last price", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		measurements, err := signal.Measure(market.Input{
			Role: "ticker",
			Ticker: kraken.TickerDataSlice{{
				Symbol:    "BTC/USD",
				Bid:       99,
				Ask:       101,
				Timestamp: time.Now().UTC(),
			}},
		}, nil)

		Convey("It should ignore the incomplete observation", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given local flow driving price", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, alphaFrames())

		Convey("It emits an endogenous Pearl category candidate", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryEndogenousAlpha)
			So(result.Metric("alphaScore"), ShouldBeGreaterThan, 0)
			So(result.Metric("uplift"), ShouldBeGreaterThan, 0)
			So(distributionSum(result), ShouldAlmostEqual, 1, 0.0001)
		})
	})

	Convey("Given a sudden liquidity void", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, liquidityShockFrames())

		Convey("It emits liquidity shock when Pearl inverts the regime", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryLiquidityShock)
			So(result.Metric("shockScore"), ShouldBeGreaterThan, 0)
			So(result.Metric("contagion"), ShouldBeGreaterThan, 0)
			So(distributionSum(result), ShouldAlmostEqual, 1, 0.0001)
		})
	})

	Convey("Given associated flow already at its peak", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, betaFrames())

		Convey("It emits systemic beta when association dominates intervention", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategorySystemicBeta)
			So(result.Metric("betaScore"), ShouldBeGreaterThan, 0)
			So(distributionSum(result), ShouldAlmostEqual, 1, 0.0001)
		})
	})

	Convey("Given unstructured local flow", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, noiseFrames())

		Convey("It emits causal noise when no driver dominates", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryCausalNoise)
			So(result.Metric("noiseScore"), ShouldBeGreaterThan, 0)
			So(distributionSum(result), ShouldAlmostEqual, 1, 0.0001)
		})
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	frames := alphaFrames()

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal := NewSignal(context.Background())
		_, _ = replay(signal, frames)
		_ = signal.Close()
	}
}

func assertMeasurements(measurements []*logic.Measurement, symbol string) int {
	for _, measurement := range measurements {
		So(measurement.Source, ShouldEqual, logic.SourceCausal)
		So(measurement.Symbol, ShouldEqual, symbol)
		So(measurement.Metric("value"), ShouldBeGreaterThan, 0)
		So(measurement.Confidence, ShouldBeGreaterThan, 0)
		So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
		So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
		So(measurement.HasDistribution(), ShouldBeTrue)
		So(measurement.DominantCategory(), ShouldNotEqual, logic.CategoryTypeNone)
	}

	return len(measurements)
}

func replay(
	signal *Signal,
	inputs []market.Input,
) (*logic.Measurement, error) {
	var result *logic.Measurement

	for _, input := range inputs {
		measurements, err := signal.Measure(input, nil)
		if err != nil {
			return nil, err
		}

		for _, measurement := range measurements {
			result = measurement
		}
	}

	return result, nil
}

func alphaFrames() []market.Input {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	price := 100.0
	frames := make([]market.Input, 0, 13)

	for index := range 12 {
		flow := 1 + float64(index)
		price *= 1 + flow*0.001
		frames = append(frames, tradeInput(
			"BTC/USD",
			"buy",
			price,
			flow,
			base.Add(time.Duration(index)*time.Second),
		))
	}

	frames = append(frames, tradeInput("BTC/USD", "buy", price, 1, base.Add(13*time.Second)))
	return frames
}

func betaFrames() []market.Input {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	price := 100.0
	frames := make([]market.Input, 0, 12)

	for index := range 12 {
		flow := 1 + float64(index)
		price *= 1 + flow*0.001
		frames = append(frames, tradeInput(
			"BTC/USD",
			"buy",
			price,
			flow,
			base.Add(time.Duration(index)*time.Second),
		))
	}

	return frames
}

func liquidityShockFrames() []market.Input {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	frames := make([]market.Input, 0, 25)

	for index := range 8 {
		price := 100 + float64(index)*0.01
		spread := 0.01 + float64(index)*0.001
		depth := 100 - float64(index)
		at := base.Add(time.Duration(index) * time.Second)

		frames = append(frames,
			tickerInput("BTC/USD", price, price-spread, price+spread, depth, depth, 0.01, at),
			bookInput("BTC/USD", price-spread, price+spread, depth, depth, at),
			tradeInput("BTC/USD", "buy", price, 1+float64(index)*0.1, at),
		)
	}

	frames = append(frames, bookInput("BTC/USD", 90, 110, 0.01, 0.01, base.Add(9*time.Second)))
	return frames
}

func noiseFrames() []market.Input {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	flows := []float64{1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10}
	returns := []float64{0.001, 0.001, -0.001, -0.001, 0.0005, 0.0005, -0.0005, -0.0005, 0.0008, 0.0008, -0.0008, -0.0008}
	price := 100.0
	frames := make([]market.Input, 0, len(flows))

	for index, flow := range flows {
		price *= 1 + returns[index]
		frames = append(frames, tradeInput(
			"BTC/USD",
			"buy",
			price,
			flow,
			base.Add(time.Duration(index)*time.Second),
		))
	}

	return frames
}

func tickerInput(
	symbol string,
	last float64,
	bid float64,
	ask float64,
	bidQty float64,
	askQty float64,
	changePct float64,
	at time.Time,
) market.Input {
	return market.Input{
		Role: "ticker",
		Ticker: kraken.TickerDataSlice{{
			Symbol:    symbol,
			Last:      last,
			Bid:       bid,
			Ask:       ask,
			BidQty:    bidQty,
			AskQty:    askQty,
			Volume:    1000,
			ChangePct: changePct,
			Timestamp: at,
		}},
	}
}

func bookInput(
	symbol string,
	bid float64,
	ask float64,
	bidQty float64,
	askQty float64,
	at time.Time,
) market.Input {
	return market.Input{
		Role: "book",
		Book: kraken.BookDataSlice{{
			Symbol:    symbol,
			Type:      "update",
			Timestamp: at,
			Bids:      []kraken.BookLevel{{Price: bid, Qty: bidQty}},
			Asks:      []kraken.BookLevel{{Price: ask, Qty: askQty}},
		}},
	}
}

func tradeInput(
	symbol string,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) market.Input {
	return market.Input{
		Role: "trade",
		Trade: kraken.TradeDataSlice{{
			Symbol:    symbol,
			Side:      side,
			Price:     price,
			Qty:       quantity,
			Timestamp: at,
		}},
	}
}

func distributionSum(measurement *logic.Measurement) float64 {
	total := 0.0

	for _, category := range causalCategories {
		total += measurement.Distribution[category]
	}

	return total
}
