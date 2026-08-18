package correlation

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given a symbol's very first ticker with no cohort history", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("AAA/USD", nil)
		at := time.Unix(1_700_000_000, 0).UTC()
		appendCorrelationTickers(market, correlationTicker("AAA/USD", 100, at))

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should emit the immature zero evidence rather than nothing", func() {
			So(measurements, ShouldHaveLength, 1)

			measurement := measurements[0]

			So(measurement.Source, ShouldEqual, types.SourceCorrelation)
			So(measurement.Symbol, ShouldEqual, "AAA/USD")
			So(measurement.At, ShouldResemble, at)
			So(measurement.Maturity, ShouldEqual, 0)
			So(measurement.Sample(types.MetricCorrelation, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricHerdScore, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricAlphaScore, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricNoiseScore, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricStressScore, types.SideNone).Raw, ShouldEqual, 0)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0)
		})
	})

	Convey("Given a fully co-moving cohort across repeated cuts", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("AAA/USD", nil)
		start := time.Unix(1_700_006_000, 0).UTC()

		for leg, prices := range [][]float64{
			{100, 200, 300},
			{110, 220, 330},
			{121, 242, 363},
			{133, 266, 399},
		} {
			at := start.Add(time.Duration(leg) * time.Second)
			appendCorrelationTickers(market,
				correlationTicker("AAA/USD", prices[0], at),
				correlationTicker("BBB/USD", prices[1], at),
				correlationTicker("CCC/USD", prices[2], at),
			)
		}

		measurements := slices.Collect(signal.Measure(market))

		Convey("It should emit one row per observed ticker and classify the herd", func() {
			So(measurements, ShouldHaveLength, 12)

			leader := measurements[len(measurements)-1]

			So(leader.Symbol, ShouldEqual, "CCC/USD")
			So(leader.Maturity, ShouldBeGreaterThan, 0)
			So(leader.Sample(types.MetricCorrelation, types.SideNone).Raw,
				ShouldBeGreaterThan, 0)
			So(leader.Sample(types.MetricHerdScore, types.SideNone).Raw,
				ShouldBeGreaterThan, 0)
			So(leader.Sample(types.MetricHerdScore, types.SideNone).Normalized,
				ShouldNotBeNil)
		})
	})
}

func TestFrame(t *testing.T) {
	Convey("Given an ineligible classification for an observed ticker", t, func() {
		signal := NewSignal(context.Background(), nil)
		at := time.Unix(1_700_007_000, 0).UTC()
		ticker := correlationTicker("AAA/USD", 100, at)

		measurement := signal.frame(
			ticker, equation.FeatureFrame{}, equation.CohortOutput{}, false,
		)

		Convey("It should carry the complete immature metric set", func() {
			So(measurement.Metrics, ShouldHaveLength, 6)
			So(measurement.Metadata["category"], ShouldEqual, 0)
			So(measurement.Metadata["energy"], ShouldEqual, 0)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0)
		})
	})
}

func correlationTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Change:    decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}

func appendCorrelationTickers(market *types.Symbol, rows ...kraken.TickerData) {
	for _, row := range rows {
		market.AppendTicker(row, types.TickerReceivers)
	}
}
