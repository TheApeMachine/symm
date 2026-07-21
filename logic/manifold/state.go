package manifold

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
Particle is the dashboard view of one focused symbol observation after the
shared Sensorium step. Cell coordinates preserve the established wire format.
*/
type Particle struct {
	Role      string  `json:"role"`
	CellX     float64 `json:"cell_x"`
	CellY     float64 `json:"cell_y"`
	CellZ     float64 `json:"cell_z"`
	Phase     float64 `json:"phase"`
	Omega     float64 `json:"omega"`
	Amplitude float64 `json:"amplitude"`
	Heat      float64 `json:"heat"`
	VelX      float64 `json:"vel_x"`
	VelY      float64 `json:"vel_y"`
	VelZ      float64 `json:"vel_z"`
	Speed     float64 `json:"speed"`
}

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
	Replay                bool              `json:"replay,omitempty"`
	Grid                  pfluid.Grid       `json:"grid,omitempty"`
	Rho                   [][]float64       `json:"rho,omitempty"`
	PsiMag2               [][]float64       `json:"psiMag2,omitempty"`
	GuidanceVelX          [][]float64       `json:"guidanceVelX,omitempty"`
	GuidanceVelZ          [][]float64       `json:"guidanceVelZ,omitempty"`
	Particles             []Particle        `json:"particles,omitempty"`
	Wave                  []pfluid.WaveMode `json:"wave,omitempty"`
	PhaseReady            bool              `json:"phaseReady"`
	PhaseReason           string            `json:"phaseReason,omitempty"`
	PhaseScan             []PhaseResponse   `json:"phaseScan,omitempty"`
}

/*
view constructs the symbol-local market metadata around a shared physical
reading. Touch notionals were multiplied exactly with Kraken decimals; this
market state and forecast contracts preserve those exact values for sizing.
*/
func (slot *symbolSlot) view(
	candidate intensityCandidate,
	outcome excitation.Outcome,
	reading pfluid.Reading,
	diagnostics pfluid.Diagnostics,
	grid pfluid.Grid,
	advanced bool,
) State {
	if !advanced {
		state := slot.last
		state.Reading = reading
		state.Replay = true
		slot.last.Reading = reading
		return state
	}

	buyIntensity, sellIntensity := intensities(outcome)
	interval := outcome.Horizon

	if interval <= 0 && !slot.last.At.IsZero() {
		interval = outcome.At.Sub(slot.last.At)
	}

	state := State{
		Source:           "manifold",
		Symbol:           candidate.symbol,
		At:               outcome.At,
		Duration:         interval,
		Epoch:            slot.epoch,
		ReferencePrice:   slot.coords.reference.Copy(),
		Spread:           slot.coords.spread,
		BuyCapacity:      slot.coords.buyCapacity.Copy(),
		SellCapacity:     slot.coords.sellCapacity.Copy(),
		InvalidReason:    Valid,
		StressAnisotropy: stressAnisotropy(outcome),
		Subdivisions:     diagnostics.Halvings + 1,
		BuyIntensity:     buyIntensity,
		SellIntensity:    sellIntensity,
		SpectralRadius:   outcome.Fit.SpectralRadius,
		Reading:          reading,
		OscillatorCount:  slot.end - slot.start,
		Grid:             grid,
	}
	slot.last = state
	return state
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
Summary removes projection payloads while retaining all scalar physics.
*/
func (state State) Summary() State {
	state.Grid = pfluid.Grid{}
	state.Rho = nil
	state.PsiMag2 = nil
	state.GuidanceVelX = nil
	state.GuidanceVelZ = nil
	state.Particles = nil
	state.Wave = nil
	state.PhaseReady = false
	state.PhaseReason = ""
	state.PhaseScan = nil

	return state
}
