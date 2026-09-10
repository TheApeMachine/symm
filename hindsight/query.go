package hindsight

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
Selector names what a query asks the record for. It is the declared question,
reported back with whatever it produced, so a reader always knows exactly what
was asked rather than inferring it from the answer.
*/
type Selector string

/*
Excursions asks for the confirmed directional moves — the spans the inspection
view draws as bars. A move qualifies on its own geometry and is confirmed only
once price retraced away from the extremum it reached.
*/
const Excursions Selector = "excursions"

/*
Query asks the record one question and hands back the tape that answers it.

Nothing is read here. The tape knows which run it speaks for and what was asked
of it, and goes to the record when it is asked for something.
*/
func Query(
	selector Selector, catalog *tables.Catalog, run RunID, policy DiscoveryPolicy,
) *Tape {
	return &Tape{selector: selector, catalog: catalog, run: run, policy: policy}
}

/*
Measurements is what the running binary held across every move the query
selected: one leg per move, and within a leg one frame per capture identity, in
the order they were captured.

The legs stay separate because the reading after a move's last is not its
successor, and anything replaying them as one sequence would learn a join that
never happened.

An identity the record stored no state for contributes no frame. A learner shown
an empty update learns that nothing happened, when in fact nothing was measured.
*/
func (tape *Tape) Measurements() [][][]*data.Measurement[float64] {
	if tape.catalog == nil || tape.selector != Excursions {
		return nil
	}
	observations, _, err := ReadObservations(
		context.Background(), tape.catalog, tape.run, 0,
	)

	if err != nil {
		tape.err = errnie.Error(err)

		return nil
	}

	return tape.MeasurementsFrom(observations)
}

/*
MeasurementsFrom derives the confirmed-move tape from observations already in
hand. It is the same projection Measurements performs, split out so a bounded
reader can accumulate pages and re-project them as each page arrives instead
of waiting for a whole run to decode.
*/
func (tape *Tape) MeasurementsFrom(
	observations []Observation,
) [][][]*data.Measurement[float64] {
	return tape.measurements(observations, false)
}

/*
MeasurementsLive derives the same tape but admits the run's final, not yet
retraced excursion. The hindsight page shows the live run's developing move
immediately; the learning loader does the same so the grid and learner lanes
have data before a retracement closes the move.
*/
func (tape *Tape) MeasurementsLive(
	observations []Observation,
) [][][]*data.Measurement[float64] {
	return tape.measurements(observations, true)
}

func (tape *Tape) measurements(
	observations []Observation, live bool,
) [][][]*data.Measurement[float64] {
	if tape.catalog == nil || tape.selector != Excursions {
		return nil
	}
	index := NewRunIndex(tape.run, observations)
	legs := make([][][]*data.Measurement[float64], 0)

	for _, summary := range index.Summaries(tape.policy) {
		for _, move := range index.Discover(summary.Symbol, tape.policy).Episodes {
			if leg := tape.move(index, summary.Symbol, move, live); len(leg) > 0 {
				legs = append(legs, leg)
			}
		}
	}

	return legs
}

/*
move is the readings held across one excursion.

The identities are the instrument's own captures at or before the extremum,
which is the only tape that could have carried its signals, walked forwards.
Historical callers pass live=false and only receive retraced (confirmed) moves;
the live loader passes true so the current run's developing move is learned
before its retracement has printed.
*/
func (tape *Tape) move(
	index *RunIndex, symbol string, move Episode, live bool,
) [][]*data.Measurement[float64] {
	endpoint := ReferencePeak

	switch move.Kind {
	case EpisodeUpwardExcursion:
	case EpisodeDownwardExcursion:
		endpoint = ReferenceTrough
	default:
		return nil
	}
	reached, named := move.Reference(endpoint)

	if (!live && !move.Confirmed) || !named {
		return nil
	}
	candidates := index.CapturesBefore(symbol, EnvelopeRef{
		Origin: reached.Capture, Ordinal: reached.Ordinal,
	}, tape.reach())
	frames := make([][]*data.Measurement[float64], 0, len(candidates))

	for at := len(candidates) - 1; at >= 0; at-- {
		payload, stored, err := tape.catalog.ReadStatePayload(
			string(candidates[at].Origin.Run),
			uint64(candidates[at].Origin.Sequence),
			candidates[at].Ordinal,
		)

		if err != nil {
			tape.err = errnie.Error(err)

			return nil
		}

		if !stored {
			continue
		}
		measurements, err := types.MeasurementsFromState(payload)

		if err != nil {
			tape.err = errnie.Error(err)

			return nil
		}

		if len(measurements) == 0 {
			continue
		}
		frames = append(frames, measurements)
	}

	return frames
}

/* reach is how far back a move's readings are gathered from its extremum. */
func (tape *Tape) reach() int {
	if tape.Reach > 0 {
		return tape.Reach
	}

	return DefaultReach
}

/*
DefaultReach is how many of an instrument's own captures a move gathers when
none is declared. It is a declared bound on the walk, reported with what it
produced, not a claim about how long a move lasts.
*/
const DefaultReach = 256

/* Error exposes what the record refused, if anything. */
func (tape *Tape) Error() error { return tape.err }
