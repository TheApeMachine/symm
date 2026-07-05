package exhaust

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var exhaustCategories = []logic.CategoryType{
	logic.CategoryMechanicalCollapse,
	logic.CategoryThermalExhaustion,
	logic.CategoryFragileExpansion,
	logic.CategoryActiveReversal,
}

func TestSignalIngestRoles(testingTB *testing.T) {
	Convey("Given an exhaust signal", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		Convey("It declares only book and trade ingest roles", func() {
			So(signal.IngestRoles(), ShouldResemble, []string{"book", "trade"})
		})

		Convey("It ignores ticker rows", func() {
			measurements, err := signal.Measure(market.Input{Role: "ticker"}, nil)

			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 0)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given book and trade rows", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		inputs := []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 20, 20, startAt(0))),
			bookInput(bookRow("BTC/USD", 100, 101, 18, 18, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 16, 16, startAt(2))),
			bookInput(bookRow("BTC/USD", 100, 101, 14, 14, startAt(3))),
			bookInput(bookRow("BTC/USD", 100, 101, 12, 12, startAt(4))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(5))),
			bookInput(bookRow("BTC/USD", 100, 101, 8, 8, startAt(6))),
			bookInput(bookRow("BTC/USD", 100, 101, 6, 6, startAt(7))),
		}
		count := 0

		for _, input := range inputs {
			measurements, err := signal.Measure(input, nil)
			So(err, ShouldBeNil)

			for _, measurement := range measurements {
				count++
				assertMeasurement(measurement, "BTC/USD")
			}
		}

		Convey("It emits typed exhaustion measurements", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given crumbling bid-side depth", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 20, 20, startAt(0))),
			bookInput(bookRow("BTC/USD", 100, 101, 18, 18, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 16, 16, startAt(2))),
			bookInput(bookRow("BTC/USD", 100, 101, 14, 14, startAt(3))),
			bookInput(bookRow("BTC/USD", 100, 101, 12, 12, startAt(4))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(5))),
			bookInput(bookRow("BTC/USD", 100, 101, 8, 8, startAt(6))),
			bookInput(bookRow("BTC/USD", 100, 101, 6, 6, startAt(7))),
		})

		Convey("It classifies mechanical collapse", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryMechanicalCollapse)
			So(result.Metric("mechanical"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given widening spread without book collapse", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(0))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(2))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(3))),
			bookInput(spreadRow("BTC/USD", 104, startAt(4))),
		})

		Convey("It classifies fragile expansion", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryFragileExpansion)
			So(result.Metric("fragile"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given support-side imbalance flipping against the position", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 20, 5, startAt(0))),
			bookInput(bookRow("BTC/USD", 100, 101, 20, 5, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 20, 5, startAt(2))),
			bookInput(bookRow("BTC/USD", 100, 101, 20, 5, startAt(3))),
			bookInput(bookRow("BTC/USD", 100, 101, 5, 20, startAt(4))),
		})

		Convey("It classifies active reversal", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryActiveReversal)
			So(result.Metric("reversal"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given fading aggressive buy pressure", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(0))),
			tradeInput(tradeRow("BTC/USD", "buy", 100, 20, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(2))),
			tradeInput(tradeRow("BTC/USD", "buy", 100, 18, startAt(3))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(4))),
			tradeInput(tradeRow("BTC/USD", "buy", 100, 16, startAt(5))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(6))),
			tradeInput(tradeRow("BTC/USD", "buy", 100, 4, startAt(7))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(8))),
			tradeInput(tradeRow("BTC/USD", "buy", 100, 1, startAt(9))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(10))),
		})

		Convey("It classifies thermal exhaustion", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryThermalExhaustion)
			So(result.Metric("thermal"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureStableBook(testingTB *testing.T) {
	Convey("Given stable book history without decay", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(0))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(1))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(2))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(3))),
			bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(4))),
		})

		Convey("It abstains instead of emitting a random-baseline contract", func() {
			So(err, ShouldBeNil)
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	inputs := []market.Input{
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(0))),
		tradeInput(tradeRow("BTC/USD", "buy", 100, 20, startAt(1))),
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(2))),
		tradeInput(tradeRow("BTC/USD", "buy", 100, 18, startAt(3))),
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(4))),
		tradeInput(tradeRow("BTC/USD", "buy", 100, 16, startAt(5))),
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(6))),
		tradeInput(tradeRow("BTC/USD", "buy", 100, 4, startAt(7))),
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(8))),
		tradeInput(tradeRow("BTC/USD", "buy", 100, 1, startAt(9))),
		bookInput(bookRow("BTC/USD", 100, 101, 10, 10, startAt(10))),
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal := NewSignal(context.Background())
		_, _ = replay(signal, inputs)
		_ = signal.Close()
	}
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

func assertMeasurement(measurement *logic.Measurement, symbol string) {
	So(measurement.Source, ShouldEqual, logic.SourceExhaustion)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Metric("value"), ShouldBeGreaterThan, 0)
	So(measurement.Confidence, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(measurement.HasDistribution(), ShouldBeTrue)
	So(exhaustCategory(measurement.DominantCategory()), ShouldBeTrue)
}

func bookInput(row kraken.BookData) market.Input {
	return market.Input{
		Role: "book",
		Book: kraken.BookDataSlice{row},
	}
}

func tradeInput(row kraken.TradeData) market.Input {
	return market.Input{
		Role:  "trade",
		Trade: kraken.TradeDataSlice{row},
	}
}

func bookRow(
	symbol string,
	bid float64,
	ask float64,
	bidQty float64,
	askQty float64,
	at time.Time,
) kraken.BookData {
	return kraken.BookData{
		Symbol:    symbol,
		Type:      "update",
		Timestamp: at,
		Bids:      []kraken.BookLevel{{Price: bid, Qty: bidQty}},
		Asks:      []kraken.BookLevel{{Price: ask, Qty: askQty}},
	}
}

func spreadRow(symbol string, ask float64, at time.Time) kraken.BookData {
	return kraken.BookData{
		Symbol:    symbol,
		Type:      "update",
		Timestamp: at,
		Bids:      []kraken.BookLevel{{Price: 100, Qty: 10}},
		Asks: []kraken.BookLevel{
			{Price: 101, Qty: 0},
			{Price: ask, Qty: 10},
		},
	}
}

func tradeRow(
	symbol string,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       quantity,
		Timestamp: at,
	}
}

func startAt(offset int) time.Time {
	return time.Date(2026, 5, 30, 12, 0, offset, 0, time.UTC)
}

func exhaustCategory(category logic.CategoryType) bool {
	for _, candidate := range exhaustCategories {
		if category == candidate {
			return true
		}
	}

	return false
}
