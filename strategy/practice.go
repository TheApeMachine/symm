package strategy

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/*
practice is the environment a replay worker acts in.

A worker is not trading. It is being shown the tape either side of a moment
that already happened — the ignition the excursion started at, and the extremum
it ended at — and asked, at every observation, what it makes of the system
state in front of it. Its whole task is to recognise those two moments from
their precursors: the tape before ignition is the entry precursor, and the tape
between ignition and extremum is the exit precursor.

Nothing about executability belongs here. There is no book, no wallet, no fee
and no size: whether a venue could have absorbed an order says nothing about
whether the worker recognised the moment, and letting it speak removed the
worker's ability to answer at all on thin instruments. The only two things a
worker can say are the two things being judged.
*/
type practice struct {
	symbol   string
	entry    int
	exit     int
	index    int
	at       time.Time
	holding  bool
	decision *agent.Decision[Action]
}

/*
Feasible offers the answer the worker is being asked for and nothing else.

The position state joins the context under the same identity the live desk
uses, so a precursor learned in practice is keyed the same way when the
accumulated agent meets it on the live tape.
*/
func (session *practice) Feasible(label string) ([]Action, []uint64, error) {
	state := []uint64{FlatPositionContext}

	if session.holding {
		return []Action{{Kind: "hold"}, {Kind: "exit", Reduce: true}},
			[]uint64{state[0] + 1}, nil
	}

	return []Action{{Kind: "wait"}, {Kind: "enter"}}, state, nil
}

/*
Execute records the call. A call is a statement about the tape, so the only
state it changes is whether this worker considers itself in the move it named.
*/
func (session *practice) Execute(decision *agent.Decision[Action]) error {
	if decision == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "practice: decision required", nil))
	}

	switch decision.Action.Kind {
	case "enter":
		session.holding = true
	case "exit":
		session.holding = false
	}
	session.decision = decision
	return nil
}

/*
Objective advances the worker's clock and reports no valuation.

A worker holds no wallet, so there is no value to mark. The reward ledger needs
a monotonic reading to advance at all; a constant one advances the clock while
saying, truthfully, that nothing was earned here. What the worker is graded on
arrives through Resolve, never through this mark.

at is the worker's processing instant, with its monotonic clock component.
Captured venue and receive times remain signal context; neither is guaranteed
to increase with capture sequence. Grades depend on tape indices, not elapsed
processing time.
*/
func (session *practice) Objective() (*reward.Mark, error) {
	return &reward.Mark{At: session.at, Version: uint64(session.index + 1)}, nil
}

/*
judge scores one call by where it was made rather than by what it would have
earned.

The tape's own geometry supplies the scale: the distance from ignition to
extremum is one leg, and that leg is the unit a call's error is measured in. A
call landing exactly on the moment scores 1, one landing a full leg away scores
-1, and the crossover sits at half a leg. Nothing here is a tuned threshold —
every quantity comes from the episode that was actually captured.

Waiting is graded as the exact opposite of calling, because on this tape they
are the same claim stated two ways: to wait at ignition is to have missed it,
and to wait far from it is to have read it correctly.
*/
func (session *practice) judge(tape fragment, index int, action Action) (float64, string) {
	if !tape.ignites() {
		// An unchanged-price span contains no moment. Staying out is the whole
		// correct answer, and any call on it is a call on nothing.
		switch action.Kind {
		case "wait", "hold":
			return 1, "no moment to call"
		default:
			return -1, "called an unchanged tape"
		}
	}
	target, moment := tape.entry, "ignition"

	if session.holding || action.Kind == "hold" || action.Kind == "exit" {
		target, moment = tape.exit, "extremum"
	}
	recognition := recognition(index, target, tape.exit-tape.entry)

	switch action.Kind {
	case "enter", "exit":
		return recognition, verdict(index, target, moment)
	default:
		return -recognition, verdict(index, target, moment)
	}
}

/*
recognition is how well an index names a moment, on the scale of the leg that
moment belongs to. It is 1 on the moment itself, 0 half a leg away, and -1 a
full leg away or further.
*/
func recognition(index, target, leg int) float64 {
	if leg <= 0 {
		return 0
	}
	error := index - target

	if error < 0 {
		error = -error
	}
	score := 1 - 2*float64(error)/float64(leg)

	return max(-1, min(1, score))
}

/* verdict states the timing in the worker's own terms for the operator. */
func verdict(index, target int, moment string) string {
	switch {
	case index == target:
		return "called " + moment
	case index < target:
		return "early for " + moment
	default:
		return "late for " + moment
	}
}
