package exhaust

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given independent exhaustion symbols without a ready book", t, func() {
		signal := &Signal{ctx: context.Background(), books: emptyBookSource{}}
		market := types.NewSymbol("AAA/USD", nil)
		market.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "buy",
			Price: *decimal.NewFromInt64(100), Qty: 1,
			TradeID: 1, Timestamp: time.Unix(1_700_001_000, 0).UTC(),
		})

		Reset(func() {
			signal.Close()
		})

		Convey("It completes each independent symbol pass with an immature reading", func() {
			err := signal.Measure(market)
			So(err, ShouldBeNil)

			readings := slices.Collect(market.MarketMeasurements("category"))
			So(readings, ShouldHaveLength, 1)
			So(readings[0].Source, ShouldEqual, types.SourceExhaustion)
			So(readings[0].Maturity, ShouldEqual, 0)
			So(readings[0].Metrics, ShouldBeEmpty)
		})
	})
}

func appendTickers(market *types.Symbol, rows ...kraken.TickerData) {
	for _, row := range rows {
		market.AppendTicker(row)
	}
}

type emptyBookSource struct{}

func (emptyBookSource) Book(_ string, read func(*spotbook.Book)) {
	read(nil)
}

func TestMeasureTrade(t *testing.T) {
	Convey("Given pressure fade after a causal adverse book-price move", t, func() {
		sample := algorithm.NewDecaySample()
		expectedSample := algorithm.NewDecaySample()
		signal := &Signal{sample: sample, decay: equation.NewDecay()}
		_, ready, _, err := sample.MeasureBook(exhaustBookAt(100, 101, 10, 10))
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		_, ready, _, err = expectedSample.MeasureBook(exhaustBookAt(100, 101, 10, 10))
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		_, _, _, err = sample.MeasureTrade(flow.TradeInput{
			Symbol: "BTC/USD", Price: 100, Quantity: 10,
			Side: flow.TradeBuy, At: time.Unix(1, 0),
		})
		So(err, ShouldBeNil)
		_, _, _, err = expectedSample.MeasureTrade(flow.TradeInput{
			Symbol: "BTC/USD", Price: 100, Quantity: 10,
			Side: flow.TradeBuy, At: time.Unix(1, 0),
		})
		So(err, ShouldBeNil)
		adverseBook := flow.BookInput{
			Symbol: "BTC/USD", TickSize: 1,
			Bids: []flow.BookLevel{
				{Price: 100, Ticks: 100, Quantity: 0},
				{Price: 99, Ticks: 99, Quantity: 10},
			},
			Asks: []flow.BookLevel{
				{Price: 101, Ticks: 101, Quantity: 0},
				{Price: 100, Ticks: 100, Quantity: 10},
			},
		}
		_, ready, _, err = sample.MeasureBook(adverseBook)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		_, ready, _, err = expectedSample.MeasureBook(adverseBook)
		So(err, ShouldBeNil)
		So(ready, ShouldBeTrue)
		at := time.Unix(2, 0)
		trade := kraken.TradeData{
			Symbol: "BTC/USD", Side: "buy", Price: *decimal.NewFromInt64(99),
			Qty: 1, TradeID: 41, Timestamp: at,
		}
		expectedInput, _, expectedMaturity, err := expectedSample.MeasureTrade(
			flow.TradeInput{
				Symbol: trade.Symbol, Price: 99, Quantity: trade.Qty,
				Side: flow.TradeBuy, At: trade.Timestamp,
			},
		)
		So(err, ShouldBeNil)
		expectedOutput, err := equation.NewDecay().Measure(expectedInput)
		So(err, ShouldBeNil)
		measurement := signal.tradeReading(trade)

		Convey("It should emit positive long thermal exhaustion only after both legs", func() {
			So(measurement, ShouldNotBeNil)
			So(measurement.At, ShouldResemble, at)
			So(measurement.Maturity, ShouldEqual, expectedMaturity)
			So(measurement.Sample(types.MetricThermal, types.SideBuy).Raw,
				ShouldEqual, expectedOutput.Long.Thermal)
			So(measurement.Sample(types.MetricThermal, types.SideSell).Raw,
				ShouldEqual, expectedOutput.Short.Thermal)
			So(measurement.Sample(types.MetricMechanical, types.SideBuy).Raw,
				ShouldEqual, expectedOutput.Long.Mechanical)
			So(measurement.Sample(types.MetricMechanical, types.SideSell).Raw,
				ShouldEqual, expectedOutput.Short.Mechanical)
		})
	})
}

func TestFrame(t *testing.T) {
	Convey("Given side-specific decay output", t, func() {
		signal := &Signal{}
		at := time.Unix(1_700_001_200, 0).UTC()
		measurement := signal.frame("BTC/USD", at, equation.DecayOutput{
			Long: equation.DecaySideOutput{
				Thermal: 0.4, Value: 0.4, Strength: 0.4, Category: 3,
			},
			Short: equation.DecaySideOutput{
				Mechanical: 0.2, Value: 0.2, Strength: 0.2, Category: 1,
			},
		}, 0.75)

		Convey("It should preserve both side families under the existing metric keys", func() {
			So(measurement, ShouldNotBeNil)
			So(measurement.Metrics, ShouldHaveLength, 17)
			So(measurement.Sample(types.MetricThermal, types.SideBuy).Raw,
				ShouldEqual, 0.4)
			So(measurement.Sample(types.MetricMechanical, types.SideSell).Raw,
				ShouldEqual, 0.2)
			So(*measurement.Sample(types.MetricThermal, types.SideBuy).Normalized,
				ShouldEqual, 0.4)
			So(measurement.Sample(types.MetricCategory, types.SideBuy).Normalized,
				ShouldBeNil)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, (0.4-0.2)/0.4)

			for _, sample := range measurement.Metrics {
				So(sample.Unit, ShouldEqual, types.UnitDimensionless)
			}
		})
	})

	Convey("Given tied long and short exhaustion hypotheses", t, func() {
		measurement := (&Signal{}).frame(
			"BTC/USD",
			time.Unix(1_700_001_201, 0).UTC(),
			equation.DecayOutput{
				Long: equation.DecaySideOutput{
					Thermal: 0.5, Strength: 0.5,
				},
				Short: equation.DecaySideOutput{
					Mechanical: 0.5, Strength: 0.5,
				},
			},
			1,
		)

		Convey("It should report zero HypothesisSeparation", func() {
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0.0)
		})
	})
}

func TestNormalizedDecayScore(t *testing.T) {
	Convey("Given the exact closed domain of every decay probability margin", t, func() {
		Convey("It should retain both boundaries and an interior score", func() {
			So(*normalizedDecayScore(0), ShouldEqual, 0.0)
			So(*normalizedDecayScore(0.375), ShouldEqual, 0.375)
			So(*normalizedDecayScore(1), ShouldEqual, 1.0)
		})

		Convey("It should reject one-ULP underflow and overflow", func() {
			So(normalizedDecayScore(math.Nextafter(0, -1)), ShouldBeNil)
			So(normalizedDecayScore(math.Nextafter(1, 2)), ShouldBeNil)
		})
	})
}

func TestValidDecayCategory(t *testing.T) {
	Convey("Given decay's nominal category domain", t, func() {
		Convey("It should accept every exact identifier", func() {
			for category := 0.0; category <= maximumDecayCategory; category++ {
				So(validDecayCategory(category), ShouldBeTrue)
			}
		})

		Convey("It should reject fractional and out-of-range identifiers", func() {
			So(validDecayCategory(0.5), ShouldBeFalse)
			So(validDecayCategory(math.Nextafter(0, -1)), ShouldBeFalse)
			So(validDecayCategory(
				math.Nextafter(maximumDecayCategory, maximumDecayCategory+1),
			), ShouldBeFalse)
		})
	})
}

func TestNormalizedDecayMetrics(t *testing.T) {
	Convey("Given one malformed probability margin in an otherwise valid output", t, func() {
		output := equation.DecayOutput{}
		output.Long.Thermal = math.Nextafter(1, 2)
		metrics, valid := normalizedDecayMetrics(output)

		Convey("It should preserve the raw audit value and reject the bundle", func() {
			So(valid, ShouldBeFalse)
			So(metrics, ShouldHaveLength, 16)
			So(metrics[types.MetricKey(
				types.MetricThermal,
				types.SideBuy,
			)].Raw, ShouldEqual, output.Long.Thermal)
			So(metrics[types.MetricKey(
				types.MetricThermal,
				types.SideBuy,
			)].Normalized, ShouldBeNil)
		})
	})
}

func exhaustBookAt(
	bidPrice, askPrice, bidQuantity, askQuantity float64,
) flow.BookInput {
	return flow.BookInput{
		Symbol:   "BTC/USD",
		TickSize: 1,
		Bids: []flow.BookLevel{
			{Price: bidPrice, Ticks: int64(bidPrice), Quantity: bidQuantity},
		},
		Asks: []flow.BookLevel{
			{Price: askPrice, Ticks: int64(askPrice), Quantity: askQuantity},
		},
	}
}

func BenchmarkFrame(b *testing.B) {
	signal := &Signal{}
	at := time.Unix(1_700_001_300, 0).UTC()
	output := equation.DecayOutput{
		Long: equation.DecaySideOutput{
			Mechanical: 0.2, Fragile: 0.3, Thermal: 0.4, Reversal: 0.1,
			Urgency: 0.3, Strength: 0.3, Value: 0.3, Category: 3,
		},
		Short: equation.DecaySideOutput{
			Mechanical: 0.1, Fragile: 0.2, Thermal: 0.3, Reversal: 0.4,
			Urgency: 0.3, Strength: 0.3, Value: 0.3, Category: 4,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.frame("BTC/USD", at, output, 1)
	}
}
