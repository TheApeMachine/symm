package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
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

		Convey("It should emit Hawkes intensity metrics from the nomagique pipeline", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)

			latest := measurements[len(measurements)-1]
			So(latest.Source, ShouldEqual, string(types.SourceHawkes))

			_, found := latest.Metrics["buy_intensity"]
			So(found, ShouldBeTrue)

			_, found = latest.Metrics["sell_intensity"]
			So(found, ShouldBeTrue)

			eventCount, found := latest.Metrics["event_count"]
			So(found, ShouldBeTrue)
			So(eventCount.Raw, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkHawkesPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("BTC/USD")
	start := time.Unix(1_700_007_000, 0).UTC()

	for index := range b.N {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		market.AppendTrade(kraken.TradeData{
			Symbol:    "BTC/USD",
			TradeID:   int64(index + 1),
			Side:      side,
			Timestamp: start.Add(time.Duration(index) * time.Second),
		})
	}

	signal := NewSignal(context.Background(), thesis)

	b.Run("run", func(b *testing.B) {
		for range b.N {
			_ = signal
		}
	})
}
