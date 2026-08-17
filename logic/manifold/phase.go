package manifold

import (
	"sort"
	"math"
	"math/cmplx"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
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
pendingDial is a universe cut awaiting its realized direction. The dial and the
mass-weighted mids of every symbol that contributed to the cut are held until
the horizon elapses on the manifold's own clock.
*/
type pendingDial struct {
	dial    geometry.PhaseDial
	at      time.Time
	cuts    int
	weights []phaseWeight
}

/*
phaseWeight is one symbol's contribution to a universe outcome: the mid at the
cut and the particle mass that symbol injected into the shared gas.
*/
type phaseWeight struct {
	symbol string
	mid    float64
	mass   float64
}

/*
realizedDirection classifies a forward log return against the observable book's
own scale.

Why:

	A dead zone is required or every cut is labelled by its last tick of noise, but
	the width cannot be a constant: the same fractional move is decisive for one
	book and inside the spread for another. The solver already accumulates the
	RMS log distance of resting orders from mid, which is that book's own measure
	of how far price has to travel to mean anything, so the dead zone is derived
	rather than chosen. The universe scale is the mass-weighted mix of those
	per-book scales, not a second invented threshold.
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
stampPhase reads the resident universe fingerprint, matures any dial whose
horizon has elapsed, sweeps the corpus, and records the result on the thesis.

The sweep queries only retained history: the current cut is excluded, so the
dial reports what past market states this one resonates with rather than
rediscovering itself at alpha zero.
*/
func (solver *Solver) stampPhase(
	thesis *types.Thesis,
	at time.Time,
	perSymbol map[string][]pmanifold.Oscillator,
) types.PhaseReading {
	solver.mature()

	if len(solver.oscillators) == 0 {
		reading := types.PhaseReading{At: at, Reason: "wave unavailable"}
		solver.recordPhase(thesis, reading)

		return reading
	}

	dial, dialErr := projectSourceDial(
		solver.oscillators,
		solver.config.GateWidthMin(),
		solver.config.GateWidthMax(),
	)

	if dialErr != nil || len(dial) == 0 {
		reading := types.PhaseReading{At: at, Reason: "no resident fingerprint"}
		solver.recordPhase(thesis, reading)

		return reading
	}

	if weights, complete := solver.observeWeights(perSymbol); complete {
		solver.pending = append(solver.pending, pendingDial{
			dial:    dial,
			at:      at,
			weights: weights,
		})
	}

	reading := solver.sweep(at, dial)
	solver.recordPhase(thesis, reading)

	return reading
}

/*
sweep rotates the resident dial through the full circle and reports the ranked
corpus at each angle. The leader is still the alignment, but the rest of the
rank is what the rotation actually saw — the geodesic, not a single winner.
*/
func (solver *Solver) sweep(
	at time.Time,
	dial geometry.PhaseDial,
) types.PhaseReading {
	reading := types.PhaseReading{At: at}

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

	reading.Responses = make([]types.PhaseResponse, 0, len(responses)*phaseScanTopK)

	for index, matches := range responses {
		if len(matches) == 0 {
			continue
		}

		for _, match := range matches {
			reading.Responses = append(reading.Responses, types.PhaseResponse{
				Angle:      solver.angles[index],
				Similarity: match.Similarity,
				ObservedAt: match.At.Format(time.RFC3339),
				Outcome:    match.Outcome,
			})
		}
	}

	if len(reading.Responses) == 0 {
		reading.Reason = "no retained response"

		return reading
	}

	reading.Ready = true

	return reading
}

/*
recordPhase stamps one sweep on the thesis so a later stage and the display
read the same reading rather than two derivations of it.
*/
func (solver *Solver) recordPhase(
	thesis *types.Thesis,
	reading types.PhaseReading,
) {
	if thesis == nil {
		return
	}

	thesis.StorePhase(reading)
}

/*
mature ages held universe dials by a cut and retains those whose horizon has
elapsed, tagged with the mass-weighted direction the observable books took.
*/
func (solver *Solver) mature() {
	if len(solver.pending) == 0 {
		return
	}

	kept := solver.pending[:0]

	for _, entry := range solver.pending {
		entry.cuts++

		if entry.cuts < phaseOutcomeHorizon {
			kept = append(kept, entry)

			continue
		}

		if len(entry.dial) != int(phaseLatticeWidth) {
			continue
		}

		outcome, complete := solver.universeOutcome(entry.weights)

		if !complete {
			continue
		}

		errnie.Error(solver.corpus.Insert(geometry.CorpusEntry[types.PhaseOutcome]{
			Dial:    entry.dial,
			At:      entry.at,
			Outcome: outcome,
		}))
	}

	solver.pending = kept
}

/*
observeWeights snapshots every contributing book's mid and injected mass.
The universe outcome is only well-defined when every cut can be priced.
*/
func (solver *Solver) observeWeights(
	perSymbol map[string][]pmanifold.Oscillator,
) ([]phaseWeight, bool) {
	if len(perSymbol) == 0 {
		return nil, false
	}

	symbols := make([]string, 0, len(perSymbol))

	for symbol := range perSymbol {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	weights := make([]phaseWeight, 0, len(perSymbol))

	for _, symbol := range symbols {
		mid := solver.midpoint(symbol)

		if !(mid > 0) {
			return nil, false
		}

		var mass float64

		for _, oscillator := range perSymbol[symbol] {
			orderMass := oscillator.Amplitude * oscillator.Amplitude

			if !(orderMass > 0) {
				continue
			}

			mass += orderMass
		}

		if !(mass > 0) {
			return nil, false
		}

		weights = append(weights, phaseWeight{
			symbol: symbol,
			mid:    mid,
			mass:   mass,
		})
	}

	return weights, true
}

/*
universeOutcome is the mass-weighted log return of every book that contributed
to the held cut, classified against those books' mass-weighted scale.
*/
func (solver *Solver) universeOutcome(weights []phaseWeight) (types.PhaseOutcome, bool) {
	if len(weights) == 0 {
		return types.PhaseOutcome{}, false
	}

	var weightedReturn float64
	var weightedScale float64
	var totalMass float64

	for _, weight := range weights {
		current := solver.midpoint(weight.symbol)

		if !(current > 0) || !(weight.mid > 0) {
			return types.PhaseOutcome{}, false
		}

		scale, scaled := solver.scale(weight.symbol)

		if !scaled {
			return types.PhaseOutcome{}, false
		}

		weightedReturn += weight.mass * (math.Log(current) - math.Log(weight.mid))
		weightedScale += weight.mass * scale
		totalMass += weight.mass
	}

	if !(totalMass > 0) {
		return types.PhaseOutcome{}, false
	}

	forward := weightedReturn / totalMass
	scale := weightedScale / totalMass

	return types.PhaseOutcome{
		Direction: realizedDirection(forward, scale),
		Return:    forward,
		Horizon:   phaseOutcomeHorizon,
	}, true
}

/*
midpoint reads one symbol's current mid. The universe outcome mixes these
mids; it does not replace them with a second invented price.
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

/*
WaveMode is one complex mode contribution on the universe phase dial.
*/
type WaveMode struct {
	Omega     float32 `json:"omega"`
	Real      float32 `json:"real"`
	Imaginary float32 `json:"imaginary"`
}

func oscillatorWave(oscillators []pmanifold.Oscillator) []WaveMode {
	wave := make([]WaveMode, len(oscillators))

	for index, oscillator := range oscillators {
		wave[index] = WaveMode{
			Omega:     float32(oscillator.Omega),
			Real:      float32(oscillator.Amplitude * math.Cos(oscillator.Phase)),
			Imaginary: float32(oscillator.Amplitude * math.Sin(oscillator.Phase)),
		}
	}

	return wave
}

func projectSourceDial(
	oscillators []pmanifold.Oscillator,
	omegaMin float64,
	omegaMax float64,
) (geometry.PhaseDial, error) {
	if len(oscillators) == 0 {
		return nil, nil
	}

	span := omegaMax - omegaMin

	if !(span > 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: omega span must be positive",
			nil,
		))
	}

	bins := int(phaseLatticeWidth)
	occupancy := make([]complex128, bins)
	wave := make([]complex128, bins)

	for _, oscillator := range oscillators {
		mass := oscillator.Amplitude * oscillator.Amplitude

		if !(mass > 0) {
			continue
		}

		bin := omegaBin(oscillator.Omega, omegaMin, span, bins)
		occupancy[bin] += cmplx.Rect(mass, oscillator.Phase)
		wave[bin] += complex(
			oscillator.Amplitude*math.Cos(oscillator.Phase),
			oscillator.Amplitude*math.Sin(oscillator.Phase),
		)
	}

	dial := make(geometry.PhaseDial, bins)
	energy := 0.0

	for index := range dial {
		dial[index] = occupancy[index] * wave[index]
		magnitude := cmplx.Abs(dial[index])

		if math.IsNaN(magnitude) || math.IsInf(magnitude, 0) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"manifold: projected dial is not finite",
				nil,
			))
		}

		energy += magnitude * magnitude
	}

	if !(energy > 0) {
		return nil, nil
	}

	return dial, nil
}

/*
omegaBin places a frequency on the fixed carrier lattice. The corpus compares
fingerprints by index, so the lattice width cannot follow however many
oscillators happened to be resident in a given cut.
*/
func omegaBin(omega, omegaMin, span float64, bins int) int {
	position := (omega - omegaMin) / span

	return min(max(int(position*float64(bins)), 0), bins-1)
}
