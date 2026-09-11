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
ReplayFragmentsFrom derives confirmed-move replay fragments with objective
boundary metadata (anchor index B and extremum index) preserved for replay.
*/
func (tape *Tape) ReplayFragmentsFrom(
	observations []Observation,
) []types.ReplayFragment {
	return tape.fragments(observations, false)
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
	fragments := tape.fragments(observations, live)
	legs := make([][][]*data.Measurement[float64], 0, len(fragments))

	for _, fragment := range fragments {
		if len(fragment.Frames) > 0 {
			legs = append(legs, fragment.Frames)
		}
	}

	return legs
}

func (tape *Tape) fragments(
	observations []Observation, live bool,
) []types.ReplayFragment {
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
	fragments := make([]types.ReplayFragment, 0, len(excursions))

	for _, window := range excursions {
		fragment := tape.fragment(index, window, decoded)

		if len(fragment.Frames) > 0 && fragment.AnchorIndex > 0 && fragment.AnchorIndex < len(fragment.Frames) {
			fragments = append(fragments, fragment)
		}

		if tape.err != nil {
			return nil
		}
	}

	return fragments
}

/* excursionWindow is one move's captures plus the equal-length precursor and aftermath. */
type excursionWindow struct {
	symbol    string
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
		symbol:    symbol,
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
fragment is the readings held across one excursion with factual boundary metadata.

An identity the record stored no measurements for contributes no frame.
Frames are produced without semantic action labels: what the learner
should do is the learner's decision, not something the record pre-writes.
The anchor and extremum indices are factual replay boundaries.
*/
func (tape *Tape) fragment(
	index *RunIndex,
	window excursionWindow,
	decoded map[tables.EnvelopeRefRow][]*data.Measurement[float64],
) types.ReplayFragment {
	held := make([][]*data.Measurement[float64], 0, len(window.captures))
	prices := make([]float64, 0, len(window.captures))
	anchorIdx := -1
	extremumIdx := -1

	for _, candidate := range window.captures {
		measurements, stored := decoded[envelopeRow(candidate)]

		if !stored || len(measurements) == 0 {
			continue
		}

		priceVal := 0.0

		if index != nil {
			observation, hasObs := index.ObservationAt(window.symbol, candidate)

			if hasObs {
				if p, ok := extractObsPrice(observation); ok {
					priceVal = p
				}
			}
		}

		if candidate == window.anchor && anchorIdx < 0 {
			anchorIdx = len(held)
		}

		if candidate == window.extremum && extremumIdx < 0 {
			extremumIdx = len(held)
		}

		held = append(held, measurements)
		prices = append(prices, priceVal)
	}

	return types.ReplayFragment{
		Frames:        held,
		Prices:        prices,
		Symbol:        window.symbol,
		AnchorIndex:   anchorIdx,
		ExtremumIndex: extremumIdx,
	}
}

func extractObsPrice(observation Observation) (float64, bool) {
	if observation.HasLast && observation.Last > 0 {
		return observation.Last, true
	}

	if observation.HasTrade && observation.TradePrice > 0 {
		return observation.TradePrice, true
	}

	if observation.HasBid && observation.Bid > 0 {
		return observation.Bid, true
	}

	return 0, false
}

/* Error exposes what the record refused, if anything. */
func (tape *Tape) Error() error { return errnie.Error(tape.err) }
