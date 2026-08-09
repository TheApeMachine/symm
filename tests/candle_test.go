package tests

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketRenderCandle(t *testing.T) {
	Convey("Given executed samples spanning two intervals", t, func() {
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{
			testtypes.NewSymbol("CANDLE/USD", 100, 71),
		})
		market := &Market{Config: config, candles: map[string]*candleState{}}
		start := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
		first := testtypes.Sample{
			Symbol: "CANDLE/USD", Last: 100, StepVolume: 2,
			Timestamp: start.Add(10 * time.Second),
		}
		second := first
		second.Last = 102
		second.StepVolume = 3
		second.Timestamp = start.Add(20 * time.Second)
		third := second
		third.Last = 99
		third.StepVolume = 1
		third.Timestamp = start.Add(time.Minute + time.Second)
		market.renderCandle(first)
		payload := market.renderCandle(second)
		frame := kraken.OHLC{}
		So(json.Unmarshal(payload, &frame), ShouldBeNil)
		rolled := kraken.OHLC{}
		So(json.Unmarshal(market.renderCandle(third), &rolled), ShouldBeNil)

		Convey("OHLC and VWAP should derive from those same trades", func() {
			So(frame.Data[0].Open, ShouldEqual, 100.0)
			So(frame.Data[0].High, ShouldEqual, 102.0)
			So(frame.Data[0].Low, ShouldEqual, 100.0)
			So(frame.Data[0].Close, ShouldEqual, 102.0)
			So(frame.Data[0].Trades, ShouldEqual, int64(2))
			So(frame.Data[0].Volume, ShouldEqual, 5.0)
			So(frame.Data[0].Vwap, ShouldEqual, 101.2)
			So(rolled.Data[0].Open, ShouldEqual, 99.0)
			So(rolled.Data[0].Trades, ShouldEqual, int64(1))
		})
	})
}

func BenchmarkMarketRenderCandle(b *testing.B) {
	config := testtypes.NewScenarioConfig([]*testtypes.Symbol{
		testtypes.NewSymbol("CANDLE/USD", 100, 71),
	})
	market := &Market{Config: config, candles: map[string]*candleState{}}
	sample := testtypes.Sample{
		Symbol: "CANDLE/USD", Last: 100, StepVolume: 1,
		Timestamp: testtypes.DefaultScenarioStart,
	}

	for b.Loop() {
		sample.Timestamp = sample.Timestamp.Add(time.Millisecond)
		_ = market.renderCandle(sample)
	}
}
