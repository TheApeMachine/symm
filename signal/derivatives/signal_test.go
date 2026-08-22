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
			state := signal.getState("BTC/USD")
			state.RecordPriceSample(time.Unix(100, 0), 65000, 65000)
			state.RecordPriceSample(time.Unix(101, 0), 65100, 65120)
			state.RecordPriceSample(time.Unix(102, 0), 65300, 65350)

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 50000000,
				Last:         decimal.NewFromFloat64(65350),
				IndexPrice:   decimal.NewFromFloat64(65300),
				Timestamp:    time.Unix(101, 0),
			})

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 55000000, // +10% OI expansion
				Last:         decimal.NewFromFloat64(65500),
				IndexPrice:   decimal.NewFromFloat64(65450),
				Timestamp:    time.Unix(102, 0),
			})

			symbol.AppendFuturesTrade(kraken.FuturesTradeData{
				Symbol:    "BTC/USD",
				ProductID: "PF_XBTUSD",
				Side:      "buy",
				Type:      "fill",
				Price:     *decimal.NewFromFloat64(65500),
				Qty:       10.0,
				Timestamp: time.Unix(102, 100),
			})

			time.Sleep(20 * time.Millisecond)

			var measurements []*nmtypes.Measurement

			for m := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, m)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(latest.Metrics[string(types.MetricLeveragedIgnition)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesOI)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesOIVelocity)], ShouldNotBeNil)
		})

		Convey("Processing a short squeeze sequence should register liquidation bursts and short squeeze score", func() {
			state := signal.getState("BTC/USD")
			state.RecordPriceSample(time.Unix(100, 0), 65000, 65000)
			state.RecordPriceSample(time.Unix(101, 0), 65200, 65250)

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 50000000,
				Last:         decimal.NewFromFloat64(65000),
				Timestamp:    time.Unix(100, 0),
			})

			symbol.AppendFuturesTicker(kraken.FuturesTickerData{
				Symbol:       "BTC/USD",
				ProductID:    "PF_XBTUSD",
				OpenInterest: 45000000, // -10% OI contraction
				Last:         decimal.NewFromFloat64(65400),
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

			time.Sleep(20 * time.Millisecond)

			var measurements []*nmtypes.Measurement

			for m := range symbol.MarketMeasurements(
				symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, m)
			}

			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(latest.Metrics[string(types.MetricShortSqueeze)], ShouldNotBeNil)
			So(latest.Metrics[string(types.MetricFuturesLiquidationBuy)].Raw, ShouldBeGreaterThan, 0)
		})

		Convey("A symbol with no futures activity should remain unblocked", func() {
			altSymbol := thesis.Symbol("UNKNOWN/USD")
			So(altSymbol.HasPendingWork(types.SourceDerivatives), ShouldBeFalse)
		})
	})
}

func BenchmarkBuildMeasurement(b *testing.B) {
	state := NewSymbolState()
	state.RecordPriceSample(time.Unix(100, 0), 65000, 65000)
	state.RecordPriceSample(time.Unix(101, 0), 65100, 65150)
	state.RecordPriceSample(time.Unix(102, 0), 65200, 65300)
	state.LastOpenInterest = 50000000
	state.OIVelocity = 0.05
	state.FuturesCVD = 1000000
	state.Basis = 0.002
	state.IndexBasis = 0.001
	state.TripartiteDiv = 0.001

	now := time.Now().UTC()

	for b.Loop() {
		_ = BuildMeasurement("derivatives", "BTC/USD", state, now)
	}
}

func BenchmarkLeadLagCorrelation(b *testing.B) {
	state := NewSymbolState()

	for i := 0; i < 50; i++ {
		t := time.Unix(int64(i), 0)
		state.RecordPriceSample(t, 65000.0+float64(i)*10.0, 65000.0+float64(i)*10.5)
	}

	for b.Loop() {
		state.updateLeadLag()
	}
}
