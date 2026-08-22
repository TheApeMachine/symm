package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestHawkesPipeline(t *testing.T) {
	Convey("Given Hawkes trade observations on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		start := time.Unix(1_700_005_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		for index, side := range []string{"buy", "sell", "buy"} {
			market.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				TradeID:   int64(index + 1),
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		measurements := drainHawkesMeasurements(market, 3)

		Convey("It should emit the typed Hawkes process metrics the UI consumes", func() {
			So(len(measurements), ShouldEqual, 3)

			latest := measurements[len(measurements)-1]
			So(latest.Source, ShouldEqual, string(types.SourceHawkes))

			expected := []string{
				types.MetricKey(types.MetricEventCount, types.SideNone),
				types.MetricKey(types.MetricEventCount, types.SideBuy),
				types.MetricKey(types.MetricEventCount, types.SideSell),
				types.MetricKey(types.MetricConditionalIntensity, types.SideBuy),
				types.MetricKey(types.MetricConditionalIntensity, types.SideSell),
				types.MetricKey(types.MetricBaselineIntensity, types.SideBuy),
				types.MetricKey(types.MetricBaselineIntensity, types.SideSell),
				types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy),
				types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell),
				types.MetricKey(types.MetricDecayRate, types.SideNone),
				types.MetricKey(types.MetricSpectralRadius, types.SideNone),
				types.MetricKey(types.MetricTotalDescendants, types.SideBuy),
			}

			for _, metric := range expected {
				_, found := latest.Metrics[metric]
				So(found, ShouldBeTrue)
			}

			eventCount := latest.Metrics[types.MetricKey(types.MetricEventCount, types.SideNone)]
			So(eventCount.Raw, ShouldEqual, 3)
			So(latest.Metrics[types.MetricKey(types.MetricSpectralRadius, types.SideNone)].Normalized, ShouldNotBeNil)
			So(latest.Metrics[types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy)].Normalized, ShouldNotBeNil)
		})

		Convey("Adversarial: Intense single-sided burst excites buy channel", func() {
			burstStart := start.Add(10 * time.Second)

			for index := range 10 {
				input := nomagique.Frame{}
				input.Put(algo.SymbolMark, 1.0)
				input.Put(nmtypes.EventTimeSec, float64(burstStart.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(index*1000))

				output, err := signal.number.Step("BURST/USD", input)
				So(err, ShouldBeNil)

				buyIntensity := output.MustGet(algo.SymbolLambdaAlpha)
				sellIntensity := output.MustGet(algo.SymbolLambdaBeta)
				So(buyIntensity, ShouldBeGreaterThan, sellIntensity)
			}
		})
	})
}

func drainHawkesMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
	readings := []*nmtypes.Measurement{}
	deadline := time.Now().Add(2 * time.Second)

	for len(readings) < expected && time.Now().Before(deadline) {
		for measurement := range symbol.MarketMeasurements(
			symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
		) {
			readings = append(readings, measurement)
		}

		if len(readings) >= expected {
			break
		}

		time.Sleep(time.Millisecond)
	}

	return readings
}

func BenchmarkHawkesPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	start := time.Unix(1_700_007_000, 0).UTC()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		input := nomagique.Frame{}
		input.Put(algo.SymbolMark, 1.0)
		input.Put(nmtypes.EventTimeSec, float64(start.Unix()))
		input.Put(nmtypes.EventTimeNsec, 0)

		_, _ = signal.number.Step("BTC/USD", input)
	}
}
