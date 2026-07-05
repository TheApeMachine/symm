package depthflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var depthflowCategories = []logic.CategoryType{
	logic.CategoryLoadedImbalance,
	logic.CategorySpoofTrap,
	logic.CategoryBookThinning,
	logic.CategoryDenseNeutrality,
}

func TestSignalIngestRoles(testingTB *testing.T) {
	Convey("Given a depthflow signal", testingTB, func() {
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
			bookInput(bookRow("BTC/USD", 20, 8, startAt(0))),
			tradeInput(tradeRow("BTC/USD", "buy", 100.5, 4, startAt(1))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(2))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(3))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(4))),
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

		Convey("It emits typed depthflow measurements", func() {
			So(count, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given bid-heavy depth confirmed by buy pressure", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 20, 8, startAt(0))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(1))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(2))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(3))),
			tradeInput(tradeRow("BTC/USD", "buy", 100.5, 4, startAt(4))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(5))),
		})

		Convey("It classifies loaded imbalance", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryLoadedImbalance)
			So(result.Metric("loadedScore"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid weight contradicted by bearish touch pressure", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		result, err := replay(signal, []market.Input{
			bookInput(bookRow("BTC/USD", 20, 8, startAt(0))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(1))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(2))),
			bookInput(bookRow("BTC/USD", 20, 8, startAt(3))),
			bookInput(spoofRow("BTC/USD", startAt(4))),
			tradeInput(tradeRow("BTC/USD", "sell", 100.5, 8, startAt(5))),
			bookInput(spoofRow("BTC/USD", startAt(6))),
		})

		Convey("It classifies spoof trap", func() {
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategorySpoofTrap)
			So(result.Metric("spoofScore"), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	inputs := []market.Input{
		bookInput(bookRow("BTC/USD", 20, 8, startAt(0))),
		bookInput(bookRow("BTC/USD", 20, 8, startAt(1))),
		bookInput(bookRow("BTC/USD", 20, 8, startAt(2))),
		bookInput(bookRow("BTC/USD", 20, 8, startAt(3))),
		tradeInput(tradeRow("BTC/USD", "buy", 100.5, 4, startAt(4))),
		bookInput(bookRow("BTC/USD", 20, 8, startAt(5))),
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
	So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(measurement.Metric("value"), ShouldBeGreaterThan, 0)
	So(measurement.Confidence, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(measurement.HasDistribution(), ShouldBeTrue)
	So(depthflowCategory(measurement.DominantCategory()), ShouldBeTrue)
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
	bidQty float64,
	askQty float64,
	at time.Time,
) kraken.BookData {
	return kraken.BookData{
		Symbol:    symbol,
		Type:      "update",
		Timestamp: at,
		Bids: []kraken.BookLevel{
			{Price: 100, Qty: bidQty},
			{Price: 99, Qty: bidQty * 0.9},
		},
		Asks: []kraken.BookLevel{
			{Price: 101, Qty: askQty},
			{Price: 102, Qty: askQty * 0.8},
		},
	}
}

func spoofRow(symbol string, at time.Time) kraken.BookData {
	return kraken.BookData{
		Symbol:    symbol,
		Type:      "update",
		Timestamp: at,
		Bids: []kraken.BookLevel{
			{Price: 100, Qty: 1},
			{Price: 99, Qty: 500},
			{Price: 98, Qty: 500},
		},
		Asks: []kraken.BookLevel{
			{Price: 101, Qty: 50},
			{Price: 102, Qty: 10},
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

func depthflowCategory(category logic.CategoryType) bool {
	for _, candidate := range depthflowCategories {
		if category == candidate {
			return true
		}
	}

	return false
}
