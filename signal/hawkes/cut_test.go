package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nhawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestOnTickerAdvancesCut(t *testing.T) {
	Convey("Given a Hawkes process that already has symbol state", t, func() {
		signal := NewSignal(t.Context(), nil)
		signal.thesis = types.NewThesis()
		base := time.Unix(1, 0)

		signal.mu.Lock()
		_, _, err := signal.process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		signal.mu.Unlock()
		So(err, ShouldBeNil)

		Convey("When a ticker arrives with no new trades", func() {
			cut := signal.onTicker(&kraken.Ticker{
				Channel: "ticker",
				Type:    "update",
				Data: []kraken.TickerData{{
					Symbol: "BTC/USD",
				}},
			})

			Convey("It still publishes a cut so cascade cadence is not trade-starved", func() {
				So(cut, ShouldNotBeNil)
				frozen, ok := cut.(*Cut)
				So(ok, ShouldBeTrue)
				So(frozen.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})

		Convey("When no symbols are warm yet", func() {
			cold := NewSignal(t.Context(), nil)
			cold.thesis = types.NewThesis()

			Convey("It does not invent a cut", func() {
				So(cold.onTicker(&kraken.Ticker{
					Channel: "ticker",
					Type:    "update",
					Data:    []kraken.TickerData{{Symbol: "ETH/USD"}},
				}), ShouldBeNil)
			})
		})
	})
}

func TestCutOutcome(t *testing.T) {
	Convey("Given a cut frozen from a live process", t, func() {
		signal := NewSignal(t.Context(), nil)
		signal.thesis = types.NewThesis()
		base := time.Unix(1, 0)

		signal.mu.Lock()
		_, _, err := signal.process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		signal.mu.Unlock()
		So(err, ShouldBeNil)

		first := signal.cut()
		firstCount, ok := first.Outcome("BTC/USD")
		So(ok, ShouldBeTrue)

		signal.mu.Lock()
		_, _, err = signal.process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base.Add(time.Second),
			Stream: nhawkes.NewArrivalStream(
				[]time.Time{base, base.Add(time.Second)},
				nil,
			),
		})
		signal.mu.Unlock()
		So(err, ShouldBeNil)

		Convey("It should keep the prior EventCount after the live process advances", func() {
			frozen, ok := first.Outcome("BTC/USD")
			So(ok, ShouldBeTrue)
			So(frozen.EventCount, ShouldEqual, firstCount.EventCount)

			live, ok := signal.Outcome("BTC/USD")
			So(ok, ShouldBeTrue)
			So(live.EventCount, ShouldBeGreaterThan, frozen.EventCount)
			So(first.SharedThesis(), ShouldEqual, signal.thesis)
			So(first.Symbols(), ShouldResemble, []string{"BTC/USD"})
		})
	})
}
