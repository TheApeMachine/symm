package hawkes

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/theapemachine/nomagique/algorithm/excitation"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainHawkesTrades(sub *types.Subscription[any]) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if trade, ok := frame.(*kraken.Trade); ok {
				out = append(out, trade.Data...)
			}
		default:
			return out
		}
	}
}

func measureHawkes(t *testing.T, state tests.MarketState, focus ...string) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	thesis.Causal.Store("signal:hawkes:sample", excitation.NewSample())
	thesis.Causal.Store("signal:hawkes:process", excitation.NewProcess())
	thesis.Causal.Store("signal:hawkes:mu", &sync.Mutex{})
	signal := &Signal{thesis: thesis, ui: make(chan []byte, 32)}
	tradeSub := market.Public.Subscribe("trade")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	signal.thesis.Tick++
	signal.Calculate(nil, drainHawkesTrades(tradeSub), nil)

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			signal.thesis.Tick++
			*into = append(*into, signal.Calculate(nil, drainHawkesTrades(tradeSub), nil)...)

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestMarketCalculate(t *testing.T) {
	Convey("Hawkes emits arrival evidence on directional market fixtures", t, func() {
		rows := measureHawkes(t, tests.MarketStateFastPump)
		So(rows, ShouldNotBeEmpty)

		foundEvents := false

		for _, measurement := range rows {
			measurement.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
				if metric == types.MetricEventCount && side == types.SideNone && sample.Raw > 0 {
					foundEvents = true
				}

				return true
			})
		}

		So(foundEvents, ShouldBeTrue)
	})
}
