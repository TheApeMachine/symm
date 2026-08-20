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

		for index, side := range []string{"buy", "sell", "buy"} {
			market.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				TradeID:   int64(index + 1),
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		go signal.Run()

		measurements := []*nmtypes.Measurement{}

		time.Sleep(50 * time.Millisecond)
		for measurement := range market.MarketMeasurements("category") {
			measurements = append(measurements, measurement)
		}

		Convey("It should emit the typed Hawkes process metrics the UI consumes", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)

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
				types.MetricKey(types.MetricDecayRate, types.SideNone),
				types.MetricKey(types.MetricSpectralRadius, types.SideNone),
				types.MetricKey(types.MetricTotalDescendants, types.SideBuy),
			}

			for _, metric := range expected {
				_, found := latest.Metrics[metric]
				So(found, ShouldBeTrue)
			}

			eventCount := latest.Metrics[types.MetricKey(types.MetricEventCount, types.SideNone)]
			So(eventCount.Raw, ShouldBeGreaterThan, 0)
			_, found := latest.Metrics["buy_intensity"]
			So(found, ShouldBeFalse)
		})
	})
}

func BenchmarkHawkesPipeline(b *testing.B) {
	signal := NewSignal(context.Background(), nil)
	start := time.Unix(1_700_007_000, 0).UTC()
	b.ReportAllocs()

	for index := range b.N {
		input := nomagique.Frame{}
		input.Put(algo.SymbolMark, markForSide([]string{"buy", "sell"}[index%2]))
		input.Put(nmtypes.EventTimeSec, float64(start.Unix()+int64(index)))
		input.Put(nmtypes.EventTimeNsec, 0)

		_, err := signal.number("BTC/USD", input)

		if err != nil {
			b.Fatal(err)
		}
	}
}
