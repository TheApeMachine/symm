package pumpdump

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer func() { _ = signal.Close() }()

		Convey("When ingest roles are requested", func() {
			roles := signal.IngestRoles()

			Convey("Then it should consume ticker, book, and trade data", func() {
				So(roles, ShouldResemble, []string{"ticker", "book", "trade"})
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer func() { _ = signal.Close() }()

		Convey("When ticker rows are measured", func() {
			measurements := replay(t, signal, tickerInputs("ALGO/USD", startAt(0)))

			Convey("Then ticker ignition measurements should be emitted", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)
				assertMeasurement(measurements[len(measurements)-1], "ALGO/USD")
				So(measurements[len(measurements)-1].Metrics["spread"], ShouldBeGreaterThan, 0)
			})
		})

		Convey("When book rows are measured", func() {
			measurements := replay(t, signal, bookInputs("MATIC/USD", startAt(0)))

			Convey("Then bookflow measurements should be emitted", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)
				assertMeasurement(measurements[len(measurements)-1], "MATIC/USD")
				So(bookCategory(dominantCategory(measurements[len(measurements)-1])), ShouldBeTrue)
			})
		})

		Convey("When trade rows are measured", func() {
			measurements := replay(t, signal, tradeInputs("MATIC/USD", startAt(0)))

			Convey("Then flow measurements should be emitted", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)
				assertMeasurement(measurements[len(measurements)-1], "MATIC/USD")
				So(tradeCategory(dominantCategory(measurements[len(measurements)-1])), ShouldBeTrue)
			})
		})
	})
}

func TestSignalMeasureTradeCategories(t *testing.T) {
	Convey("Given controlled pumpdump trade rows", t, func() {
		type tradeTick struct {
			side  string
			price float64
			qty   float64
		}

		type tradeCase struct {
			name         string
			ticks        []tradeTick
			wantCategory types.CategoryType
			wantScore    string
		}

		cases := []tradeCase{
			{
				name: "absorption",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 100, qty: 1},
				},
				wantCategory: types.CategoryHiddenAbsorption,
				wantScore:    "absorption",
			},
			{
				name: "drive",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "buy", price: 101, qty: 1},
					{side: "buy", price: 102, qty: 1},
					{side: "buy", price: 103, qty: 1},
				},
				wantCategory: types.CategoryAggressiveDrive,
				wantScore:    "drive",
			},
			{
				name: "balance",
				ticks: []tradeTick{
					{side: "buy", price: 100, qty: 1},
					{side: "sell", price: 100.1, qty: 1},
					{side: "buy", price: 100.2, qty: 1},
					{side: "sell", price: 100.3, qty: 1},
				},
				wantCategory: types.CategoryStochasticBalance,
				wantScore:    "balance",
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s trade flow", testCase.name), func() {
				trade := NewTrade()
				var result *types.Measurement
				base := startAt(0)

				for index, tick := range testCase.ticks {
					row := tradeRow(
						"CONTROL/USD",
						tick.side,
						tick.price,
						tick.qty,
						base.Add(time.Duration(index)*time.Second),
					)
					measurements, err := trade.Measure(row)
					So(err, ShouldBeNil)

					if len(measurements) > 0 {
						result = measurements[len(measurements)-1]
					}
				}

				Convey("Then the intended category should dominate", func() {
					So(result, ShouldNotBeNil)
					So(result.Source, ShouldEqual, types.SourcePumpDump)
					So(result.Symbol, ShouldEqual, "CONTROL/USD")
					So(dominantCategory(result), ShouldEqual, testCase.wantCategory)
					So(result.Metrics[testCase.wantScore], ShouldBeGreaterThan, 0)
					So(result.EntryBaseline, ShouldBeGreaterThan, 0)
					So(result.ExitBaseline, ShouldBeGreaterThan, 0)
				})
			})
		}
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	inputs := tickerInputs("ALGO/USD", startAt(0))

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal[any](context.Background())
		_ = replay(b, signal, inputs)
		_ = signal.Close()
	}
}

func replay(
	t testing.TB,
	signal *Signal[any],
	inputs []any,
) []*types.Measurement {
	t.Helper()

	measurements := make([]*types.Measurement, 0)
	for _, input := range inputs {
		out, err := signal.Measure(input, nil)
		if err != nil {
			t.Fatal(err)
		}

		measurements = append(measurements, out...)
	}

	return measurements
}

func tickerInputs(symbol string, base time.Time) []any {
	inputs := make([]any, 0, 32)

	for index := range 24 {
		row := tickerRow(
			symbol,
			1000+float64(index)*10,
			10000+float64(index)*100,
			base.Add(time.Duration(index)*time.Second),
		)
		inputs = append(inputs, row)
	}

	spike := tickerRow(symbol, 5000, 20000, base.Add(25*time.Second))
	inputs = append(inputs, spike)

	return inputs
}

func bookInputs(symbol string, base time.Time) []any {
	return []any{
		bookRow(symbol, 20, 8, base),
		bookRow(symbol, 20, 8, base.Add(time.Second)),
		bookRow(symbol, 20, 8, base.Add(2*time.Second)),
		bookRow(symbol, 20, 8, base.Add(3*time.Second)),
		bookRow(symbol, 20, 8, base.Add(5*time.Second)),
	}
}

func tradeInputs(symbol string, base time.Time) []any {
	rows := make(kraken.TradeDataSlice, 0, 8)
	for index := range 8 {
		rows = append(rows, tradeRow(
			symbol,
			"buy",
			100+float64(index),
			1,
			base.Add(time.Duration(index)*time.Second),
		))
	}

	inputs := make([]any, 0, len(rows))
	for _, row := range rows {
		inputs = append(inputs, row)
	}

	return inputs
}

func assertMeasurement(measurement *types.Measurement, symbol string) {
	So(measurement.Source, ShouldEqual, types.SourcePumpDump)
	So(measurement.Symbol, ShouldEqual, symbol)
	So(measurement.At.IsZero(), ShouldBeFalse)
	So(dominantConfidence(measurement), ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(hasCategories(measurement), ShouldBeTrue)
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

func startAt(seconds int) time.Time {
	return time.Date(2026, 5, 30, 12, 0, seconds, 0, time.UTC)
}

func bookCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryLoadedImbalance,
		types.CategorySpoofTrap,
		types.CategoryBookThinning,
		types.CategoryDenseNeutrality:
		return true
	default:
		return false
	}
}

func tradeCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryHiddenAbsorption,
		types.CategoryAggressiveDrive,
		types.CategoryStochasticBalance,
		types.CategoryVolumeStarvation:
		return true
	default:
		return false
	}
}
