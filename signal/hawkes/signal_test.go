package hawkes

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given Hawkes trade observations on one symbol", t, func() {
		ui := make(chan []byte, 3)
		signal := NewSignal(context.Background(), nil, ui)
		market := types.NewSymbol("BTC/USD", nil)
		start := time.Unix(1_700_005_000, 0).UTC()

		for index, side := range []string{"buy", "sell", "buy"} {
			market.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				TradeID:   int64(index + 1),
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		measurements := signal.Measure(market)

		Convey("It should emit excitation metrics from nomagique", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(signal.process.Symbols(), ShouldResemble, []string{"BTC/USD"})
			So(latest.Source, ShouldEqual, types.SourceHawkes)
			So(latest.Symbol, ShouldEqual, "BTC/USD")
			So(latest.Sample(types.MetricEventCount, types.SideNone).Raw,
				ShouldBeGreaterThan, 0)
			So(latest.Sample(types.MetricEventCount, types.SideBuy).Raw,
				ShouldBeGreaterThanOrEqualTo, 0)
			So(latest.Sample(types.MetricEventCount, types.SideSell).Raw,
				ShouldBeGreaterThanOrEqualTo, 0)
		})
	})

	Convey("Given trades for two independent symbols", t, func() {
		signal := NewSignal(context.Background(), nil, nil)
		market := types.NewSymbol("AAA/USD", nil)
		start := time.Unix(1_700_006_000, 0).UTC()
		market.AppendTrade(kraken.TradeData{Symbol: "AAA/USD", Side: "buy", Timestamp: start})
		market.AppendTrade(kraken.TradeData{Symbol: "BBB/USD", Side: "sell", Timestamp: start})

		measurements := signal.Measure(market)

		Convey("It should keep each symbol's estimator state apart", func() {
			So(measurements, ShouldHaveLength, 2)
			So(signal.process.Symbols(), ShouldResemble,
				[]string{"AAA/USD", "BBB/USD"})
		})
	})
}

func TestSeenTrade(t *testing.T) {
	Convey("Given Hawkes arrivals sharing a timestamp", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_006_500, 0).UTC()
		first := kraken.TradeData{Symbol: "BTC/USD", TradeID: 1, Timestamp: at}
		second := kraken.TradeData{Symbol: "BTC/USD", TradeID: 2, Timestamp: at}
		regressed := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 3, Timestamp: at.Add(-time.Nanosecond),
		}

		Convey("It should admit each identity once and reject replay or temporal regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(second), ShouldBeFalse)
			signal.commitTrade(second)
			So(signal.seenTrade(second), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), nil, nil)
	market := types.NewSymbol("BTC/USD", nil)
	start := time.Unix(1_700_005_000, 0).UTC()
	iteration := 0

	b.ReportAllocs()

	for b.Loop() {
		side := "buy"

		if iteration%2 != 0 {
			side = "sell"
		}

		market.AppendTrade(kraken.TradeData{
			Symbol:    "BTC/USD",
			TradeID:   int64(iteration + 1),
			Side:      side,
			Timestamp: start.Add(time.Duration(iteration) * time.Second),
		})
		signal.Measure(market)
		iteration++
	}
}
