package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

/*
excursionAt is a DOGE/USD excursion of class role entering at entryTick and
exiting at exitTick, bought at entryAsk and sold at exitBid.
*/
func excursionAt(role string, entryTick, exitTick uint64, entryAsk, exitBid float64) *data.Value {
	value := &data.Value{
		Origin: data.ExcursionOrigin, Role: role, Scope: "DOGE/USD", Epoch: 7, Tick: exitTick, SeqIdx: exitTick,
	}
	value.FormatPrefix()

	fields := map[string]float64{
		"a_idx": 1, "a_tick": 1, "b_idx": float64(entryTick), "b_tick": float64(entryTick),
		"c_idx": float64(exitTick), "c_tick": float64(exitTick), "d_idx": float64(exitTick), "d_tick": float64(exitTick),
		"b_price": entryAsk, "c_price": exitBid, "b_ask": entryAsk, "c_bid": exitBid, "b_at": 0, "c_at": 60e9,
		"entry_idx": float64(entryTick), "entry_tick": float64(entryTick),
		"exit_idx": float64(exitTick), "exit_tick": float64(exitTick),
		"entry_ask": entryAsk, "exit_bid": exitBid, "entry_at": 0, "exit_at": 60e9,
	}

	vector := make([]float64, 0, len(data.ExcursionKeys))

	for _, key := range data.ExcursionKeys {
		vector = append(vector, fields[key])
	}

	data.AppendVector(value, vector, data.ExcursionKeys...)

	return value
}

func TestTrainerFunnel(t *testing.T) {
	Convey("Given a symbol tape whose frames 10..40 carry a path only at 20 and 40", t, func() {
		trainer := &Trainer{price: dogePrice()}
		tape := &symbolTape{frames: []frameRecord{
			{tick: 10}, {tick: 20, path: "R01/R02"}, {tick: 30}, {tick: 40, path: "R03"},
			{tick: 50}, {tick: 60}, {tick: 70},
		}}
		tallies := make(map[string]*pathTally)
		stats := make(map[string]*pathStats)
		funnel := &trainFunnel{}

		tally := func(excursion *data.Value, horizon int) {
			funnel.excursions++
			So(trainer.tally("DOGE/USD", tape, excursion, horizon, tallies, stats, funnel), ShouldBeNil)
		}

		Convey("Each excursion lands in exactly one stage and votes only with a path", func() {
			tally(excursionAt("up", 5, 30, 1, 2), 8)    // before the first frame
			tally(excursionAt("up", 70, 70, 1, 2), 1)   // frames 60..70 carry no path
			tally(excursionAt("up", 20, 40, 1, 2), 8)   // profitable: enter at R01/R02, exit at R03
			tally(excursionAt("up", 30, 50, 2, 1), 8)   // losing up: noop at R01/R02
			tally(excursionAt("down", 45, 60, 2, 1), 8) // down: noop at R03

			So(funnel.excursions, ShouldEqual, 5)
			So(funnel.noFrame, ShouldEqual, 1)
			So(funnel.noPath, ShouldEqual, 1)
			So(funnel.withPath, ShouldEqual, 3)
			So(funnel.noFrame+funnel.noPath+funnel.withPath, ShouldEqual, funnel.excursions)
			So(funnel.enters, ShouldEqual, 1)
			So(funnel.noops, ShouldEqual, 2)
			So(funnel.exits, ShouldEqual, 1)

			// R01/R02: 1 enter + 1 noop, a tie noop takes; R03: 1 exit + 1
			// noop elects exit.
			So(*tallies["R01/R02"], ShouldResemble, pathTally{ups: 1, downs: 1})
			So(tallies["R01/R02"].majority(), ShouldEqual, "noop")
			So(*tallies["R03"], ShouldResemble, pathTally{downs: 1, exits: 1})
			So(tallies["R03"].majority(), ShouldEqual, "exit")
		})
	})

	Convey("Given path tallies, majority elects one action, noop on an enter/noop tie, none on an enter/exit tie", t, func() {
		So((&pathTally{ups: 2, downs: 1}).majority(), ShouldEqual, "enter")
		So((&pathTally{downs: 1}).majority(), ShouldEqual, "noop")
		So((&pathTally{ups: 1, exits: 1}).majority(), ShouldEqual, "")
		So((&pathTally{ups: 1, downs: 1}).majority(), ShouldEqual, "noop")
		So((&pathTally{ups: 2, exits: 2, downs: 1}).majority(), ShouldEqual, "")
	})
}
