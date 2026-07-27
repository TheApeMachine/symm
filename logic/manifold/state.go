package manifold

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
PhaseOutcome is the DMT attractor classification that was actually available
for a focused market state when its resident field snapshot was retained. Its
support and ambiguity make the categorical phase sectors auditable.
*/
type PhaseOutcome struct {
	Symbol     string  `json:"symbol"`
	Class      string  `json:"class"`
	Confidence float64 `json:"confidence"`
	Ambiguous  bool    `json:"ambiguous"`
	Cohort     uint64  `json:"cohort"`
}

/*
PhaseResponse is the strongest outcome-labeled historical corpus response at
one global phase rotation of the current resident wave. Similarity remains
signed so destructive interference is not presented as affinity.
*/
type PhaseResponse struct {
	Angle      float64      `json:"angle"`
	Similarity float64      `json:"similarity"`
	ObservedAt time.Time    `json:"observedAt"`
	Outcome    PhaseOutcome `json:"outcome"`
}

/*
State combines symbol-local market facts with a reading of the one shared
physical field advanced by the complete observed universe.
*/
type State struct {
	Source                string            `json:"source"`
	Symbol                string            `json:"symbol"`
	At                    time.Time         `json:"at"`
	Duration              time.Duration     `json:"duration"`
	Epoch                 uint64            `json:"epoch"`
	ReferencePrice        *decimal.Decimal  `json:"referencePrice"`
	Spread                float64           `json:"spread"`
	BuyCapacity           *decimal.Decimal  `json:"buyCapacity"`
	SellCapacity          *decimal.Decimal  `json:"sellCapacity"`
	InvalidReason         string            `json:"invalidReason,omitempty"`
	StressAnisotropy      float64           `json:"stressAnisotropy"`
	Subdivisions          uint32            `json:"subdivisions"`
	BuyIntensity          float64           `json:"buyIntensity"`
	SellIntensity         float64           `json:"sellIntensity"`
	SpectralRadius        float64           `json:"spectralRadius"`
	Reading               pfluid.Reading    `json:"reading"`
	OscillatorCount       int               `json:"oscillatorCount"`
	SharedOscillatorCount int               `json:"sharedOscillatorCount"`
	Grid                  pfluid.Grid       `json:"grid,omitempty"`
	Display               []byte            `json:"-"`
	DisplayWidth          int               `json:"-"`
	DisplayHeight         int               `json:"-"`
	RhoOccupied           int               `json:"rhoOccupied,omitempty"`
	PsiOccupied           int               `json:"psiOccupied,omitempty"`
	RhoMax                float64           `json:"rhoMax,omitempty"`
	PsiMax                float64           `json:"psiMax,omitempty"`
	Wave                  []pfluid.WaveMode `json:"wave,omitempty"`
	PhaseReady            bool              `json:"phaseReady"`
	PhaseReason           string            `json:"phaseReason,omitempty"`
	PhaseScan             []PhaseResponse   `json:"phaseScan,omitempty"`
	Replay                bool              `json:"replay,omitempty"`
}

/*
view constructs the symbol-local market metadata around a shared physical
reading. Touch notionals use exact Kraken decimals for sizing contracts.
appended is true when this tick ingested a new book/Hawkes sample; otherwise
the prior touch metadata is kept and only the shared reading + clock advance.
Every physics step advances the symbol's own observation clock — never the
process wall clock — so resonance chronology stays aligned with Hawkes epochs.
*/
func (slot *symbolSlot) view(
	candidate intensityCandidate,
	outcome excitation.Outcome,
	reading pfluid.Reading,
	diagnostics pfluid.Diagnostics,
	grid pfluid.Grid,
	appended bool,
) State {
	if !appended {
		state := slot.last
		state.Reading = reading
		state.Subdivisions = diagnostics.Halvings + 1
		// Microsecond bump keeps chronology strict without racing ahead of the
		// next Hawkes epoch timestamp on the market clock.
		state.At, state.Duration = slot.stepClock(
			state.At.Add(time.Microsecond),
			state.Duration,
		)
		slot.last = state
		return state
	}

	buyIntensity, sellIntensity := intensities(outcome)
	interval := outcome.Horizon

	if interval <= 0 && !slot.last.At.IsZero() {
		interval = outcome.At.Sub(slot.last.At)
	}

	at, interval := slot.stepClock(outcome.At, interval)

	state := State{
		Source:           "manifold",
		Symbol:           candidate.symbol,
		At:               at,
		Duration:         interval,
		Epoch:            slot.epoch,
		ReferencePrice:   candidate.reference,
		Spread:           candidate.spread,
		BuyCapacity:      candidate.buyCapacity,
		SellCapacity:     candidate.sellCapacity,
		InvalidReason:    Valid,
		StressAnisotropy: stressAnisotropy(outcome),
		Subdivisions:     diagnostics.Halvings + 1,
		BuyIntensity:     buyIntensity,
		SellIntensity:    sellIntensity,
		SpectralRadius:   outcome.Fit.SpectralRadius,
		Reading:          reading,
		// OscillatorCount is filled by paint from the post-merge resident count.
		Grid: grid,
	}

	if candidate.reference == nil || candidate.buyCapacity == nil ||
		candidate.sellCapacity == nil || candidate.spread <= 0 {
		state.InvalidReason = "no executable touch"
	}

	slot.last = state
	return state
}

/*
stepClock returns a strictly progressing observation time for one physics step.
Hawkes can admit a new EventCount at an unchanged wall clock; the cut clock or a
one-microsecond bump keeps resonance/causal chronology ordered.
*/
func (slot *symbolSlot) stepClock(
	candidate time.Time,
	interval time.Duration,
) (time.Time, time.Duration) {
	at := candidate

	if at.IsZero() && !slot.last.At.IsZero() {
		at = slot.last.At.Add(time.Microsecond)
	}

	if !slot.last.At.IsZero() && !at.After(slot.last.At) {
		at = slot.last.At.Add(time.Microsecond)

		if interval <= 0 {
			interval = time.Microsecond
		}
	}

	if interval <= 0 && !slot.last.At.IsZero() {
		interval = at.Sub(slot.last.At)
	}

	if interval <= 0 {
		interval = time.Microsecond
	}

	return at, interval
}

/*
IsFinite reports whether the complete scalar state is physically admissible
for downstream causal and predictive models.
*/
func (state State) IsFinite() bool {
	if state.At.IsZero() || state.Epoch == 0 {
		return false
	}

	for _, value := range []float64{
		state.StressAnisotropy,
		state.Spread,
		state.BuyIntensity,
		state.SellIntensity,
		state.SpectralRadius,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return state.ReferencePrice != nil && state.ReferencePrice.Sign() > 0 &&
		state.BuyCapacity != nil && state.BuyCapacity.Sign() > 0 &&
		state.SellCapacity != nil && state.SellCapacity.Sign() > 0 &&
		state.Spread > 0 &&
		state.Reading.IsFinite()
}

/*
GasReady reports whether a finite shared-domain state grounds this symbol.
*/
func (state State) GasReady() bool {
	return state.InvalidReason == Valid && state.IsFinite()
}

/*
Summary removes display and wave payloads while retaining all scalar physics.
*/
func (state State) Summary() State {
	state.Grid = pfluid.Grid{}
	state.Display = nil
	state.DisplayWidth = 0
	state.DisplayHeight = 0
	state.RhoOccupied = 0
	state.PsiOccupied = 0
	state.RhoMax = 0
	state.PsiMax = 0
	state.Wave = nil
	state.PhaseReady = false
	state.PhaseReason = ""
	state.PhaseScan = nil

	return state
}
