package manifold

import (
	"math"
	"time"

	mgrbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/types"
)

/*
phaseScanAngles is how many global rotations the dial is evaluated at across the
full circle. The overlaps are computed once per corpus entry and rotated
analytically, so this count costs one complex multiply per entry per angle and
does not re-encode anything.
*/
const phaseScanAngles = 72

/*
phaseScanTopK is how many corpus entries contribute at each angle. Only the
leading match is drawn, but the rank is what makes the leader meaningful.
*/
const phaseScanTopK = 16

/*
phaseCorpusCapacity bounds the retained history. The corpus is a ring, so the
oldest cut is evicted rather than the scan growing without limit.
*/
const phaseCorpusCapacity = 2048

/*
phaseOutcomeHorizon is how many manifold cuts a dial waits before its realized
direction is known. The manifold advances on its own clock, so the horizon is
counted in its own cuts rather than borrowed from another stage.
*/
const phaseOutcomeHorizon = 8

/*
phaseCorpusMinimum is how many stored cuts are required before a scan is
published as ready. Below it the leading response is an artifact of having
almost nothing to compare against.
*/
const phaseCorpusMinimum = 32

/*
pendingDial is a cut awaiting its realized direction. The dial and the mid it
was observed at are held until the horizon elapses on the manifold's own clock.
*/
type pendingDial struct {
	dial geometry.PhaseDial
	at   time.Time
	mid  float64
	cuts int
}

/*
realizedDirection classifies a forward log return against the symbol's own book
scale.

Why:

	A dead zone is required or every cut is labelled by its last tick of noise, but
	the width cannot be a constant: the same fractional move is decisive for one
	symbol and inside the spread for another. The tokenizer already accumulates the
	RMS log distance of resting orders from mid, which is that symbol's own measure
	of how far price has to travel to mean anything, so the dead zone is derived
	rather than chosen.
*/
func realizedDirection(forward, scale float64) string {
	if !(scale > 0) || math.Abs(forward) <= scale {
		return "flat"
	}

	if forward > 0 {
		return "up"
	}

	return "down"
}

/*
stampPhase reads the resident fingerprint, matures any dial whose horizon has
elapsed, sweeps the corpus, and records the result on both the wire row and the
thesis.

The sweep queries only retained history: the current cut is excluded, so the
dial reports what past market states this one resonates with rather than
rediscovering itself at alpha zero.
*/
func (solver *Solver) stampPhase(
	thesis *types.Thesis,
	row datura.Map[any],
	symbol string,
	at time.Time,
	particles []pfluid.Particle,
) {
	mid := solver.midpoint(symbol)
	solver.mature(symbol, mid)

	wave, waveErr := solver.domain.Wave()

	if waveErr != nil {
		solver.recordPhase(thesis, row, types.PhaseReading{
			Symbol: symbol, At: at, Reason: "wave unavailable",
		})

		return
	}

	row["wave"] = wave
	dial, dialErr := solver.domain.SourceDial(particles)

	if dialErr != nil || len(dial) == 0 {
		solver.recordPhase(thesis, row, types.PhaseReading{
			Symbol: symbol, At: at, Reason: "no resident fingerprint",
		})

		return
	}

	if mid > 0 {
		solver.pending[symbol] = append(solver.pending[symbol], pendingDial{
			dial: dial,
			at:   at,
			mid:  mid,
		})
	}

	reading := solver.sweep(symbol, at, dial)
	solver.recordPhase(thesis, row, reading)
}

/*
sweep rotates the resident dial through the full circle and reports the leading
retained response at each angle.
*/
func (solver *Solver) sweep(
	symbol string,
	at time.Time,
	dial geometry.PhaseDial,
) types.PhaseReading {
	reading := types.PhaseReading{Symbol: symbol, At: at}

	if solver.corpus.Size() < phaseCorpusMinimum {
		reading.Reason = "retaining history"

		return reading
	}

	responses, err := solver.corpus.ScanPhasesExcluding(
		dial, solver.angles, phaseScanTopK, at,
	)

	if err != nil {
		reading.Reason = err.Error()

		return reading
	}

	reading.Responses = make([]types.PhaseResponse, 0, len(responses))

	for index, matches := range responses {
		if len(matches) == 0 {
			continue
		}

		leader := matches[0]
		reading.Responses = append(reading.Responses, types.PhaseResponse{
			Angle:      solver.angles[index],
			Similarity: leader.Similarity,
			ObservedAt: leader.At.Format(time.RFC3339),
			Outcome:    leader.Outcome,
		})
	}

	if len(reading.Responses) == 0 {
		reading.Reason = "no retained response"

		return reading
	}

	reading.Ready = true

	return reading
}

/*
recordPhase stamps one sweep on the thesis and mirrors it onto the wire row, so
a later stage and the display read the same reading rather than two derivations
of it.
*/
func (solver *Solver) recordPhase(
	thesis *types.Thesis,
	row datura.Map[any],
	reading types.PhaseReading,
) {
	if thesis != nil {
		stored, found := thesis.Symbols.Load(reading.Symbol)

		if found {
			stored.(*types.Symbol).Phase.Store(reading.Symbol, reading)
		}
	}

	row["phaseReady"] = reading.Ready
	row["phaseReason"] = reading.Reason

	if reading.Ready {
		row["phaseScan"] = reading.Responses
	}
}

/*
mature ages one symbol's held dials by a cut and retains those whose horizon has
elapsed, tagged with the direction price took over that span.
*/
func (solver *Solver) mature(symbol string, mid float64) {
	held := solver.pending[symbol]

	if len(held) == 0 || !(mid > 0) {
		return
	}

	scale, scaled := solver.tokenizer.Scale(symbol)
	kept := held[:0]

	for _, entry := range held {
		entry.cuts++

		if entry.cuts < phaseOutcomeHorizon {
			kept = append(kept, entry)

			continue
		}

		if !scaled {
			continue
		}

		forward := math.Log(mid) - math.Log(entry.mid)

		errnie.Error(solver.corpus.Insert(geometry.CorpusEntry[types.PhaseOutcome]{
			Dial: entry.dial,
			At:   entry.at,
			Outcome: types.PhaseOutcome{
				Symbol:    symbol,
				Direction: realizedDirection(forward, scale),
				Return:    forward,
				Horizon:   phaseOutcomeHorizon,
			},
		}))
	}

	solver.pending[symbol] = kept
}

/*
midpoint reads the symbol's current mid, which is the only outside quantity the
phase corpus needs and the same book the particles were tokenized from.
*/
func (solver *Solver) midpoint(symbol string) float64 {
	if solver.api == nil {
		return 0
	}

	var midpoint float64
	solver.api.Book(symbol, func(managed *mgrbook.Book) {
		if managed != nil {
			midpoint = managed.Midpoint().Float64()
		}
	})

	return midpoint
}
