package leadlag

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainLeadlag(sub *types.Subscription[any]) []*kraken.Ticker {
	out := make([]*kraken.Ticker, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if ticker, ok := frame.(*kraken.Ticker); ok {
				out = append(out, ticker)
			}
		default:
			return out
		}
	}
}

func measureLeadlag(
	t *testing.T,
	state tests.MarketState,
	focus ...string,
) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	thesis := types.NewThesis()
	signal := &Signal{ui: make(chan []byte, 32)}
	tickerSub := market.Public.Subscribe("ticker")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	for _, ticker := range drainLeadlag(tickerSub) {
		for _, row := range ticker.Data {
			thesis.Tickers.Store(row.Symbol, row)
		}
		thesis.Tick++
		signal.Measure(thesis)
	}

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			for _, ticker := range drainLeadlag(tickerSub) {
				for _, row := range ticker.Data {
					thesis.Tickers.Store(row.Symbol, row)
				}
				thesis.Tick++
				*into = append(*into, signal.Measure(thesis)...)
			}

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestCalculate(t *testing.T) {
	Convey("Leadlag emits non-zero coordination evidence on directional cohort tapes", t, func() {
		metrics := []types.MetricType{
			types.MetricStrength,
			types.MetricSync,
		}

		baseline := tests.PeakMeasurements(
			measureLeadlag(t, tests.MarketStateBaseline),
			types.SourceLeadLag,
			metrics,
		)
		pump := tests.PeakMeasurements(
			measureLeadlag(t, tests.MarketStateFastPump),
			types.SourceLeadLag,
			metrics,
		)

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(pump[types.MetricStrength][symbol], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricStrength][symbol])
		}
	})
}
