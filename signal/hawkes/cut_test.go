package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nhawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

func TestOnTickerAdvancesCut(t *testing.T) {
	Convey("Given a Hawkes process that already has symbol state", t, func() {
		thesis := types.NewThesis()
		statePlanner := &strategy.Planner{Thesis: thesis}
		signal := NewSignal(t.Context(), nil, statePlanner, nil)
		found, _ := thesis.Causal.Load("signal:hawkes:process")
		process := found.(*excitation.Process)
		base := time.Unix(1, 0)

		_, _, err := process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		So(err, ShouldBeNil)

		Convey("When a ticker arrives with no new trades", func() {
			signal.onTicker(&kraken.Ticker{
				Channel: "ticker",
				Type:    "update",
				Data: []kraken.TickerData{{
					Symbol: "BTC/USD",
				}},
			})
			cut := signal.cut()

			Convey("It still publishes a cut so cascade cadence is not trade-starved", func() {
				So(cut, ShouldNotBeNil)
				So(cut.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})

		Convey("When no symbols are warm yet", func() {
			cold := NewSignal(t.Context(), nil, &strategy.Planner{Thesis: types.NewThesis()}, nil)

			Convey("It does not invent a cut", func() {
				cold.onTicker(&kraken.Ticker{
					Channel: "ticker",
					Type:    "update",
					Data:    []kraken.TickerData{{Symbol: "ETH/USD"}},
				})
				So(cold.cut().Symbols(), ShouldBeEmpty)
			})
		})
	})
}

func TestCutOutcome(t *testing.T) {
	Convey("Given a cut frozen from a live process", t, func() {
		thesis := types.NewThesis()
		statePlanner := &strategy.Planner{Thesis: thesis}
		signal := NewSignal(t.Context(), nil, statePlanner, nil)
		found, _ := thesis.Causal.Load("signal:hawkes:process")
		process := found.(*excitation.Process)
		base := time.Unix(1, 0)

		_, _, err := process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		So(err, ShouldBeNil)

		first := signal.cut()
		firstCount, ok := first.Outcome("BTC/USD")
		So(ok, ShouldBeTrue)

		_, _, err = process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base.Add(time.Second),
			Stream: nhawkes.NewArrivalStream(
				[]time.Time{base, base.Add(time.Second)},
				nil,
			),
		})
		So(err, ShouldBeNil)

		Convey("It should keep the prior EventCount after the live process advances", func() {
			frozen, ok := first.Outcome("BTC/USD")
			So(ok, ShouldBeTrue)
			So(frozen.EventCount, ShouldEqual, firstCount.EventCount)

			live, ok := signal.Outcome("BTC/USD")
			So(ok, ShouldBeTrue)
			So(live.EventCount, ShouldBeGreaterThan, frozen.EventCount)
			So(first.SharedThesis(), ShouldEqual, thesis)
			So(first.Symbols(), ShouldResemble, []string{"BTC/USD"})
		})
	})
}
