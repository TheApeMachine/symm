package hawkes

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nhawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

func TestOnTickerAdvancesCut(t *testing.T) {
	Convey("Given a Hawkes process that already has symbol state", t, func() {
		thesis := types.NewThesis()
		thesis.Causal.Store("signal:hawkes:sample", excitation.NewSample())
		thesis.Causal.Store("signal:hawkes:process", excitation.NewProcess())
		thesis.Causal.Store("signal:hawkes:mu", &sync.Mutex{})
		signal := &Signal{}
		found, _ := thesis.Causal.Load("signal:hawkes:process")
		process := found.(*excitation.Process)
		base := time.Unix(1, 0)

		_, _, err := process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		So(err, ShouldBeNil)

		Convey("When the shared thesis has no new trades", func() {
			cut := signal.cut(thesis)

			Convey("It still publishes a cut so cascade cadence is not trade-starved", func() {
				So(cut, ShouldNotBeNil)
				So(cut.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})

		Convey("When no symbols are warm yet", func() {
			coldThesis := types.NewThesis()
			coldThesis.Causal.Store("signal:hawkes:sample", excitation.NewSample())
			coldThesis.Causal.Store("signal:hawkes:process", excitation.NewProcess())
			coldThesis.Causal.Store("signal:hawkes:mu", &sync.Mutex{})
			cold := &Signal{}

			Convey("It does not invent a cut", func() {
				So(cold.cut(coldThesis).Symbols(), ShouldBeEmpty)
			})
		})
	})
}

func TestCutOutcome(t *testing.T) {
	Convey("Given a cut frozen from a live process", t, func() {
		thesis := types.NewThesis()
		thesis.Causal.Store("signal:hawkes:sample", excitation.NewSample())
		thesis.Causal.Store("signal:hawkes:process", excitation.NewProcess())
		thesis.Causal.Store("signal:hawkes:mu", &sync.Mutex{})
		signal := &Signal{}
		found, _ := thesis.Causal.Load("signal:hawkes:process")
		process := found.(*excitation.Process)
		base := time.Unix(1, 0)

		_, _, err := process.Measure(excitation.Input{
			Symbol:  "BTC/USD",
			Horizon: base,
			Stream:  nhawkes.NewArrivalStream([]time.Time{base}, nil),
		})
		So(err, ShouldBeNil)

		first := signal.cut(thesis)
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

			live, ok := signal.Outcome(thesis, "BTC/USD")
			So(ok, ShouldBeTrue)
			So(live.EventCount, ShouldBeGreaterThan, frozen.EventCount)
			So(first.SharedThesis(), ShouldEqual, thesis)
			So(first.Symbols(), ShouldResemble, []string{"BTC/USD"})
		})
	})
}
