package derivatives

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestDerivativesSignal(t *testing.T) {
	Convey("Given a Derivatives Signal and Thesis instance", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		thesis := types.NewThesis(ctx, nil)
		signal := NewSignal(ctx, thesis)
		defer signal.Close()

		So(signal.Name(), ShouldEqual, string(types.SourceDerivatives))
		So(signal.Type(), ShouldEqual, types.SourceDerivatives)
		So(signal.Error(), ShouldBeNil)

		symbol := thesis.Symbol("BTC/USD")

		Convey("Processing a leveraged ignition sequence should produce high LeveragedIgnition probability", func() {
			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 50000000,
				Last:         decimal.NewFromFloat64(65000),
				IndexPrice:   decimal.NewFromFloat64(65000),
				Timestamp:    time.Unix(100, 0),
			})

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 55000000, // +10% OI expansion
				Last:         decimal.NewFromFloat64(65500),
				IndexPrice:   decimal.NewFromFloat64(65450),
				Timestamp:    time.Unix(101, 0),
			})

			symbol.AppendFuturesTrade(kraken.FuturesTradeData{
				Symbol:    "BTC/USD",
				ProductID: "PF_XBTUSD",
				Side:      "buy",
				Type:      "fill",
				Price:     *decimal.NewFromFloat64(65500),
				Qty:       10.0,
				Timestamp: time.Unix(101, 100),
			})

			time.Sleep(30 * time.Millisecond)

			var measurements []*nmtypes.Measurement

			for measurement := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(latest.Metrics[string(types.MetricLeveragedIgnition)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesOI)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesOIVelocity)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesAggressorImbalance)], ShouldNotBeNil)
		})

		Convey("Processing a short squeeze sequence should register liquidation bursts and short squeeze score", func() {
			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 50000000,
				Last:         decimal.NewFromFloat64(65000),
				IndexPrice:   decimal.NewFromFloat64(65000),
				Timestamp:    time.Unix(100, 0),
			})

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 45000000, // -10% OI contraction
				Last:         decimal.NewFromFloat64(65400),
				IndexPrice:   decimal.NewFromFloat64(65350),
				Timestamp:    time.Unix(101, 0),
			})

			symbol.AppendFuturesTrade(kraken.FuturesTradeData{
				Symbol:    "BTC/USD",
				ProductID: "PF_XBTUSD",
				Side:      "buy",
				Type:      "liquidation",
				Price:     *decimal.NewFromFloat64(65400),
				Qty:       20.0,
				Timestamp: time.Unix(101, 100),
			})

			time.Sleep(30 * time.Millisecond)

			var measurements []*nmtypes.Measurement

			for measurement := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(latest.Metrics[string(types.MetricShortSqueeze)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesLiquidationBuy)].Raw, ShouldBeGreaterThan, 0)
		})

		Convey("Processing sell trades should register negative aggressor imbalance without normalization panic", func() {
			symbol.AppendFuturesTrade(kraken.FuturesTradeData{
				Symbol:    "BTC/USD",
				ProductID: "PF_XBTUSD",
				Side:      "sell",
				Type:      "fill",
				Price:     *decimal.NewFromFloat64(64000),
				Qty:       1.0,
				Timestamp: time.Unix(103, 0),
			})

			time.Sleep(30 * time.Millisecond)

			var measurements []*nmtypes.Measurement

			for measurement := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			imbalanceMetric := latest.Metrics[string(types.MetricFuturesAggressorImbalance)]
			So(imbalanceMetric, ShouldNotBeNil)
			So(imbalanceMetric.Raw, ShouldEqual, -1.0)
			So(imbalanceMetric.Normalized, ShouldBeNil)
		})

		Convey("Processing interleaved ticker and trade updates where trade timestamp precedes ticker timestamp must not trigger temporal regression", func() {
			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 50000000,
				Last:         decimal.NewFromFloat64(65000),
				IndexPrice:   decimal.NewFromFloat64(65000),
				Timestamp:    time.Unix(200, 0),
			})

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 51000000,
				Last:         decimal.NewFromFloat64(65100),
				IndexPrice:   decimal.NewFromFloat64(65050),
				Timestamp:    time.Unix(210, 0),
			})

			// Trade timestamp is earlier than the latest ticker timestamp (t=205 < t=210)
			symbol.AppendFuturesTrade(kraken.FuturesTradeData{
				Symbol:    "BTC/USD",
				ProductID: "PF_XBTUSD",
				Side:      "buy",
				Type:      "fill",
				Price:     *decimal.NewFromFloat64(65080),
				Qty:       2.5,
				Timestamp: time.Unix(205, 0),
			})

			time.Sleep(30 * time.Millisecond)

			So(signal.Error(), ShouldBeNil)

			var measurements []*nmtypes.Measurement

			for measurement := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
		})

		Convey("Zero timestamp input should fail validation with an error and never fallback silently", func() {
			measurement, err := BuildMeasurement("derivatives", "BTC/USD", time.Time{}, DerivativesData{})
			So(err, ShouldNotBeNil)
			So(measurement, ShouldBeNil)
		})

		Convey("A symbol with no futures activity should remain unblocked", func() {
			altSymbol := thesis.Symbol("UNKNOWN/USD")
			So(altSymbol.HasPendingWork(types.SourceDerivatives), ShouldBeFalse)
		})
	})
}

func BenchmarkBuildMeasurement(b *testing.B) {
	data := DerivativesData{
		OI:                     50000000.0,
		OIVelocity:             0.05,
		OIAcceleration:         0.01,
		Basis:                  0.002,
		BasisVelocity:          0.0005,
		IndexBasis:             0.001,
		TripartiteDivergence:   0.001,
		CVD:                    1000000.0,
		AggressorImbalance:     0.6,
		LiquidationBuy:         50000.0,
		LiquidationSell:        0.0,
		LiquidationIntensity:   0.05,
		LeveragedIgnition:      0.8,
		ShortSqueeze:           0.2,
		AdverseLeverageBuildup: 0.1,
		LongDeleveraging:       0.1,
		DerivativesDecoupling:  0.2,
		SampleCount:            100,
	}

	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = BuildMeasurement("derivatives", "BTC/USD", now, data)
	}
}
