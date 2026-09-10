package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
)

/* tape builds a fragment whose ignition and extremum sit where a test needs them. */
func practiceTape(length, entry, exit int) fragment {
	return fragment{
		observations: make([]hindsight.Observation, length),
		entry:        entry,
		exit:         exit,
	}
}

func TestPracticeFeasible(t *testing.T) {
	Convey("A worker is offered the call it is being asked for and nothing else", t, func() {
		session := &practice{}
		actions, state, err := session.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(actions, ShouldResemble, []Action{{Kind: "wait"}, {Kind: "enter"}})
		So(state, ShouldResemble, []uint64{FlatPositionContext})

		Convey("Calling an entry changes what it can say next, and the context with it", func() {
			So(session.Execute(&agent.Decision[Action]{Action: Action{Kind: "enter"}}), ShouldBeNil)
			actions, state, err := session.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, []Action{{Kind: "hold"}, {Kind: "exit", Reduce: true}})
			So(state, ShouldResemble, []uint64{FlatPositionContext + 1})

			Convey("And calling the exit returns it to watching", func() {
				So(session.Execute(&agent.Decision[Action]{
					Action: Action{Kind: "exit", Reduce: true},
				}), ShouldBeNil)
				actions, _, _ := session.Feasible("BTC/USD")
				So(actions, ShouldResemble, []Action{{Kind: "wait"}, {Kind: "enter"}})
			})
		})

		Convey("Its clock advances while it reports no valuation of its own", func() {
			session.index = 7
			mark, err := session.Objective()
			So(err, ShouldBeNil)
			So(mark.Version, ShouldEqual, 8)
			So(mark.Value, ShouldEqual, 0)
		})
	})
}

func TestPracticeJudge(t *testing.T) {
	Convey("A call is scored by where it was made, not by what it would have earned", t, func() {
		// Ignition at 10, extremum at 20: the leg this tape measures error in.
		tape := practiceTape(40, 10, 20)
		flat := &practice{}
		held := &practice{holding: true}

		Convey("Naming the moment exactly is entirely right", func() {
			value, verdict := flat.judge(tape, 10, Action{Kind: "enter"})
			So(value, ShouldEqual, 1)
			So(verdict, ShouldEqual, "called ignition")

			value, verdict = held.judge(tape, 20, Action{Kind: "exit", Reduce: true})
			So(value, ShouldEqual, 1)
			So(verdict, ShouldEqual, "called extremum")
		})

		Convey("Waiting through the moment is exactly as wrong as calling it was right", func() {
			called, _ := flat.judge(tape, 10, Action{Kind: "enter"})
			waited, verdict := flat.judge(tape, 10, Action{Kind: "wait"})
			So(waited, ShouldEqual, -called)
			So(verdict, ShouldEqual, "called ignition")
		})

		Convey("Error grows on the scale of the tape's own leg", func() {
			// Half a leg from the moment is the crossover; a full leg is wholly wrong.
			half, verdict := flat.judge(tape, 5, Action{Kind: "enter"})
			So(half, ShouldEqual, 0)
			So(verdict, ShouldEqual, "early for ignition")

			whole, verdict := flat.judge(tape, 20, Action{Kind: "enter"})
			So(whole, ShouldEqual, -1)
			So(verdict, ShouldEqual, "late for ignition")

			// A quarter of a leg out is half right, either side of the moment.
			early, _ := flat.judge(tape, 8, Action{Kind: "enter"})
			late, _ := flat.judge(tape, 12, Action{Kind: "enter"})
			So(early, ShouldAlmostEqual, 0.6)
			So(late, ShouldAlmostEqual, 0.6)
		})

		Convey("Waiting far from the moment is correct behaviour", func() {
			value, _ := flat.judge(tape, 30, Action{Kind: "wait"})
			So(value, ShouldEqual, 1)

			value, _ = held.judge(tape, 0, Action{Kind: "hold"})
			So(value, ShouldEqual, 1)
		})

		Convey("A tape with no moment in it can only be waited through", func() {
			quiet := practiceTape(40, -1, -1)
			So(quiet.ignites(), ShouldBeFalse)

			value, verdict := flat.judge(quiet, 12, Action{Kind: "wait"})
			So(value, ShouldEqual, 1)
			So(verdict, ShouldEqual, "no moment to call")

			value, verdict = flat.judge(quiet, 12, Action{Kind: "enter"})
			So(value, ShouldEqual, -1)
			So(verdict, ShouldEqual, "called an unchanged tape")
		})

		Convey("An entry is judged against ignition even once it has been called", func() {
			// The call is graded from the state it was made in, so entering is
			// never scored against the extremum it has not reached yet.
			value, verdict := flat.judge(tape, 20, Action{Kind: "enter"})
			So(verdict, ShouldEqual, "late for ignition")
			So(value, ShouldEqual, -1)
		})
	})
}
