package depthflow

import (
	"encoding/json"
	"testing"

	bookflow "github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func drainDepthTrades(sub *types.Subscription[any]) []kraken.TradeData {
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

func drainDepthBooks(sub *types.Subscription[any]) []kraken.BookData {
	out := make([]kraken.BookData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if book, ok := frame.(*kraken.Book); ok {
				out = append(out, book.Data...)
			}
		default:
			return out
		}
	}
}

func measureDepthflow(t *testing.T, state tests.MarketState, focus ...string) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	sample, err := bookflow.NewSample(256)
	So(err, ShouldBeNil)
	signal := &Signal{
		thesis:   types.NewThesis(),
		sample:   sample,
		bookflow: equation.NewBookflow(),
		ui:       make(chan []byte, 32),
	}
	tradeSub := market.Public.Subscribe("trade")
	bookSub := market.Public.Subscribe("book")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)

	signal.thesis.Tick++
	signal.Calculate(nil, drainDepthTrades(tradeSub), drainDepthBooks(bookSub))

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			signal.thesis.Tick++
			*into = append(*into, signal.Calculate(
				nil,
				drainDepthTrades(tradeSub),
				drainDepthBooks(bookSub),
			)...)

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestCalculate(t *testing.T) {
	Convey("Depthflow distinguishes thin, loaded, and spoofed book states", t, func() {
		metrics := []types.MetricType{
			types.MetricThinScore,
			types.MetricLoadedScore,
			types.MetricSpoofScore,
		}

		baseline := tests.PeakMeasurements(measureDepthflow(t, tests.MarketStateBaseline), types.SourceDepthFlow, metrics)
		thin := tests.PeakMeasurements(measureDepthflow(t, tests.MarketStateThinLiquidity, "SIM1/USD"), types.SourceDepthFlow, metrics)
		loaded := tests.PeakMeasurements(measureDepthflow(t, tests.MarketStateLoadedLiquidity, "SIM1/USD"), types.SourceDepthFlow, metrics)
		spoof := tests.PeakMeasurements(measureDepthflow(t, tests.MarketStateSpoofLiquidity, "SIM1/USD"), types.SourceDepthFlow, metrics)

		So(thin[types.MetricThinScore]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricThinScore]["SIM1/USD"])
		So(loaded[types.MetricLoadedScore]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricLoadedScore]["SIM1/USD"])
		So(spoof[types.MetricSpoofScore]["SIM1/USD"], ShouldBeGreaterThanOrEqualTo, baseline[types.MetricSpoofScore]["SIM1/USD"])
	})
}
