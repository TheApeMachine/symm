package hindsight

import (
	"context"
	"strconv"

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
hand. Discovery runs on those market facts; the numerical frames are then
read from the precursor witnesses those facts name, in one scan of the run.
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
	excursions := tape.excursions(index, live)

	if len(excursions) == 0 {
		return nil
	}
	decoded, err := tape.decode(identities(excursions))

	if err != nil {
		tape.err = errnie.Error(err)

		return nil
	}
	legs := make([][][]*data.Measurement[float64], 0, len(excursions))

	for _, window := range excursions {
		if leg := tape.frames(window, decoded); len(leg) > 0 {
			legs = append(legs, leg)
		}

		if tape.err != nil {
			return nil
		}
	}

	return legs
}

/* excursionWindow is one move's captures plus the equal-length precursor and aftermath. */
type excursionWindow struct {
	captures  []EnvelopeRef
	anchor    EnvelopeRef
	extremum  EnvelopeRef
	kind      EpisodeKind
	magnitude float64
	excursion float64
}

/*
excursions is each move's capture window, in capture order, before any payload
is read. Collecting the identities first is what lets the record be opened once
for the whole tape instead of once per capture.
*/
func (tape *Tape) excursions(index *RunIndex, live bool) []excursionWindow {
	collected := make([]excursionWindow, 0)

	for _, summary := range index.Summaries(tape.policy) {
		for _, move := range index.Discover(summary.Symbol, tape.policy).Episodes {
			if window := tape.window(index, summary.Symbol, move, live); len(window.captures) > 0 {
				collected = append(collected, window)
			}
		}
	}

	return collected
}

/*
window is the instrument's own captures covering one excursion: the move
itself, the same length of tape before the anchor (the precursor), and the
same length after the extremum (the retracement that confirmed it).

Historical callers pass live=false and only receive retraced moves; the live
loader passes true so the current run's developing move is learned before its
retracement has printed.
*/
func (tape *Tape) window(
	index *RunIndex, symbol string, move Episode, live bool,
) excursionWindow {
	endpoint := ReferencePeak

	switch move.Kind {
	case EpisodeUpwardExcursion:
	case EpisodeDownwardExcursion:
		endpoint = ReferenceTrough
	default:
		return excursionWindow{}
	}
	anchor, hasAnchor := move.Reference(ReferenceAnchor)
	extremum, hasExtremum := move.Reference(endpoint)

	if (!live && !move.Confirmed) || !hasAnchor || !hasExtremum {
		return excursionWindow{}
	}
	from := EnvelopeRef{Origin: anchor.Capture, Ordinal: anchor.Ordinal}
	through := EnvelopeRef{Origin: extremum.Capture, Ordinal: extremum.Ordinal}

	return excursionWindow{
		captures:  index.CapturesAround(symbol, from, through),
		anchor:    from,
		extremum:  through,
		kind:      move.Kind,
		magnitude: move.Magnitude(),
		excursion: move.ObservedExcursion,
	}
}

func identities(excursions []excursionWindow) []tables.EnvelopeRefRow {
	seen := make(map[tables.EnvelopeRefRow]struct{})
	wanted := make([]tables.EnvelopeRefRow, 0)

	for _, window := range excursions {
		for _, candidate := range window.captures {
			identity := envelopeRow(candidate)

			if _, have := seen[identity]; have {
				continue
			}
			seen[identity] = struct{}{}
			wanted = append(wanted, identity)
		}
	}

	return wanted
}

func envelopeRow(ref EnvelopeRef) tables.EnvelopeRefRow {
	return tables.EnvelopeRefRow{
		Run:      string(ref.Origin.Run),
		Sequence: int64(ref.Origin.Sequence),
		Ordinal:  int64(ref.Ordinal),
	}
}

/*
decode reads the numerical witness at each requested identity.

The dedicated tape is the precursor artifact. A coordinate that only has a
sampled full-state witness still yields that state's measurements. Payload
bytes are decoded in the scan and not retained.
*/
func (tape *Tape) decode(
	wanted []tables.EnvelopeRefRow,
) (map[tables.EnvelopeRefRow][]*data.Measurement[float64], error) {
	held := make(map[tables.EnvelopeRefRow][]*data.Measurement[float64], len(wanted))
	err := tape.catalog.EachWitnessPayload(
		context.Background(), string(tape.run), "precursor", wanted,
		func(identity tables.EnvelopeRefRow, payload []byte) error {
			return keepMeasurements(held, identity, payload)
		},
	)

	if err != nil {
		return nil, err
	}
	missing := missingIdentities(wanted, held)

	if len(missing) == 0 {
		return held, nil
	}

	if err := tape.catalog.EachWitnessPayload(
		context.Background(), string(tape.run), "state", missing,
		func(identity tables.EnvelopeRefRow, payload []byte) error {
			return keepMeasurements(held, identity, payload)
		},
	); err != nil {
		return nil, err
	}

	return held, nil
}

func keepMeasurements(
	held map[tables.EnvelopeRefRow][]*data.Measurement[float64],
	identity tables.EnvelopeRefRow,
	payload []byte,
) error {
	measurements, err := types.MeasurementsFromState(payload)

	if err != nil {
		return err
	}

	if len(measurements) == 0 {
		return nil
	}
	held[identity] = measurements

	return nil
}

func missingIdentities(
	wanted []tables.EnvelopeRefRow,
	have map[tables.EnvelopeRefRow][]*data.Measurement[float64],
) []tables.EnvelopeRefRow {
	missing := make([]tables.EnvelopeRefRow, 0)

	for _, identity := range wanted {
		if _, stored := have[identity]; stored {
			continue
		}
		missing = append(missing, identity)
	}

	return missing
}

/*
frames is the readings held across one excursion, in capture order.

An identity the record stored no measurements for contributes no frame. Each
frame is named with the moment the move itself occupies: the precursor before
the anchor, the run into the extremum, the extremum, and the aftermath.
*/
func (tape *Tape) frames(
	window excursionWindow,
	decoded map[tables.EnvelopeRefRow][]*data.Measurement[float64],
) [][]*data.Measurement[float64] {
	held := make([][]*data.Measurement[float64], 0, len(window.captures))

	// Friction hurdle: 2 * 0.008 = 0.016 (1.6% round trip at 0.8% taker fee tier).
	// An excursion must clear round-trip friction to qualify as an opportunity.
	// If magnitude is unrecorded (<= 0), we don't disqualify synthetic test frames.
	clearsFriction := window.magnitude <= 0 || window.magnitude > 0.016

	anchorIdx := -1
	extremumIdx := -1

	for i, candidate := range window.captures {
		if candidate == window.anchor {
			anchorIdx = i
		}

		if candidate == window.extremum {
			extremumIdx = i
		}
	}

	for idx, candidate := range window.captures {
		measurements, stored := decoded[envelopeRow(candidate)]

		if !stored || len(measurements) == 0 {
			continue
		}

		moment, grade := evaluateMoment(candidate, idx, anchorIdx, extremumIdx, window, clearsFriction)
		stampMoment(measurements, moment, grade)
		held = append(held, measurements)
	}

	return held
}

func evaluateMoment(
	candidate EnvelopeRef,
	idx, anchorIdx, extremumIdx int,
	window excursionWindow,
	clearsFriction bool,
) (string, float64) {
	if !clearsFriction {
		return "wait", 0.0
	}

	suffix := "_long"

	if window.kind == EpisodeDownwardExcursion {
		suffix = "_short"
	}

	if causalCmp(candidate, window.anchor) < 0 {
		u := 0.0

		if anchorIdx > 0 {
			u = float64(idx) / float64(anchorIdx)
		}

		// Mature convergence window: last 25% of precursor right before anchor
		if u >= 0.75 {
			grade := 1.0

			if window.magnitude > 0.016 {
				margin := window.magnitude - 0.016

				if margin < 0.01 {
					grade = 0.5 + 50.0*margin
				}
			}

			return "enter" + suffix, grade
		}

		// Early precursor coiling: premature entry gets lower grade
		return "enter" + suffix, 0.3 + 0.4*u
	}

	if causalCmp(candidate, window.extremum) < 0 {
		return "hold" + suffix, 0.8
	}

	if causalCmp(candidate, window.extremum) == 0 {
		return "exit" + suffix, 1.0
	}

	return "wait", 0.0
}

func stampMoment(measurements []*data.Measurement[float64], moment string, grade ...float64) {
	gradeStr := ""

	if len(grade) > 0 {
		gradeStr = strconv.FormatFloat(grade[0], 'f', 4, 64)
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string, 2)
		}

		measurement.Provenance["moment"] = moment

		if gradeStr != "" {
			measurement.Provenance["grade"] = gradeStr
		}
	}
}

/* Error exposes what the record refused, if anything. */
func (tape *Tape) Error() error { return tape.err }
