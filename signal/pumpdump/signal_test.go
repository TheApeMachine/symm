package pumpdump

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
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

		Convey("When a mid-stream volume+price pump is injected", func() {
			calm := replay(t, signal, shapedTickerRows(nil))

			pumped := NewSignal[any](context.Background())
			defer func() { _ = pumped.Close() }()

			spiked := replay(t, pumped, shapedTickerRows(
				func(frames iter.Seq[tests.Frame]) iter.Seq[tests.Frame] {
					return tests.Spike(frames, 16, 1.25, 8)
				},
			))

			Convey("Then the signal's volume lift exceeds the calm baseline", func() {
				So(len(spiked), ShouldBeGreaterThan, 0)
				So(spiked[len(spiked)-1].Source, ShouldEqual, types.SourcePumpDump)
				So(peakMetric(spiked, "rvol"), ShouldBeGreaterThan, peakMetric(calm, "rvol"))
			})
		})

		Convey("When book rows are measured", func() {
			measurements := replay(t, signal, bookInputs("MATIC/USD", startAt(0)))

			Convey("Then bookflow measurements should be emitted", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)
				measurement := measurements[len(measurements)-1]
				category := types.CategoryTypeNone
				confidence := 0.0

				for _, categoryRow := range measurement.Categories {
					if categoryRow.Confidence <= confidence {
						continue
					}

					category = categoryRow.Type
					confidence = categoryRow.Confidence
				}

				assertMeasurement(measurement, "MATIC/USD")
				So(bookCategory(category), ShouldBeTrue)
			})
		})

		Convey("When trade rows are measured", func() {
			measurements := replay(t, signal, tradeInputs("MATIC/USD", startAt(0)))

			Convey("Then flow measurements should be emitted", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)
				measurement := measurements[len(measurements)-1]
				category := types.CategoryTypeNone
				confidence := 0.0

				for _, categoryRow := range measurement.Categories {
					if categoryRow.Confidence <= confidence {
						continue
					}

					category = categoryRow.Type
					confidence = categoryRow.Confidence
				}

				assertMeasurement(measurement, "MATIC/USD")
				So(tradeCategory(category), ShouldBeTrue)
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

					category := types.CategoryTypeNone
					confidence := 0.0

					for _, categoryRow := range result.Categories {
						if categoryRow.Confidence <= confidence {
							continue
						}

						category = categoryRow.Type
						confidence = categoryRow.Confidence
					}

					So(category, ShouldEqual, testCase.wantCategory)
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

func shapedTickerRows(shape func(iter.Seq[tests.Frame]) iter.Seq[tests.Frame]) []any {
	fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)
	frames := fixture.Frames()

	if shape != nil {
		frames = shape(frames)
	}

	inputs := make([]any, 0, 32)

	for frame := range frames {
		ticker := kraken.Ticker{}

		if err := sonic.Unmarshal(frame.Payload, &ticker); err != nil {
			panic(err)
		}

		for _, row := range ticker.Data {
			inputs = append(inputs, row)
		}
	}

	return inputs
}

func peakMetric(measurements []*types.Measurement, key string) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if value := measurement.Metrics[key]; value > peak {
			peak = value
		}
	}

	return peak
}

func tickerInputs(_ string, _ time.Time) []any {
	fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)
	inputs := make([]any, 0, 32)

	for payload := range fixture.Generate() {
		frame := kraken.Ticker{}

		if err := sonic.Unmarshal(payload, &frame); err != nil {
			panic(err)
		}

		for _, row := range frame.Data {
			inputs = append(inputs, row)
		}
	}

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
	rows := make([]kraken.TradeData, 0, 8)
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

	confidence := 0.0

	for _, categoryRow := range measurement.Categories {
		if categoryRow.Confidence > confidence {
			confidence = categoryRow.Confidence
		}
	}

	So(confidence, ShouldBeGreaterThan, 0)
	So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
	So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
	So(len(measurement.Categories), ShouldBeGreaterThan, 0)
}

func bookRow(
	symbol string,
	bidQty float64,
	askQty float64,
	at time.Time,
) kraken.BookData {
	return kraken.BookData{
		Symbol:         symbol,
		Type:           "update",
		PriceIncrement: *decimal.NewFromInt64(1),
		Timestamp:      at,
		Bids: []kraken.BookLevel{
			{Price: *decimal.NewFromInt64(100), Qty: bidQty},
			{Price: *decimal.NewFromInt64(99), Qty: bidQty * 0.9},
		},
		Asks: []kraken.BookLevel{
			{Price: *decimal.NewFromInt64(101), Qty: askQty},
			{Price: *decimal.NewFromInt64(102), Qty: askQty * 0.8},
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
		Price:     *decimal.NewFromFloat64(price),
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
