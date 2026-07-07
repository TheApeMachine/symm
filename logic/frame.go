package logic

import (
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

type Batch struct {
	Actions   []*Action
	Manifold  []*ManifoldFrame
	Resonance []*ResonanceFrame
	Causal    []*CausalFrame
}

/*
Momentum collapses the per-symbol field frames into a single "is this move still
alive" score per symbol. It blends the manifold drive (momentum + guidance speed)
with resonance energy and flow — the same upward-energy the entry read is built
on — so a held position can be followed while the process keeps exciting itself
and released once that energy decays, independent of raw price jitter.

The score is unnormalised on purpose: the exit compares it against its own peak
per position, so only the relative decay matters, not an absolute scale.

Scores are taken as the MAX per symbol across that symbol's frames, not summed. A
symbol can appear more than once per cycle — one frame per measurement source, and
a sparse fallback frame on an early-bail source (price_zero) alongside a full frame
from another source. Max collapses those to the strongest live reading, so neither
duplicate full frames double-count nor a sparse fallback shadows a full frame.
*/
func (batch Batch) Momentum() map[string]float64 {
	manifold := make(map[string]float64)

	for _, frame := range batch.Manifold {
		if frame == nil {
			continue
		}

		// GuidanceSpeed is the manifold's advective drive; Momentum is the
		// net directional push. Both climb while the move is igniting and
		// fade as it stalls.
		score := math.Abs(frame.Momentum) + math.Abs(frame.Reading.GuidanceSpeed)
		manifold[frame.Symbol] = math.Max(manifold[frame.Symbol], score)
	}

	resonance := make(map[string]float64)

	for _, frame := range batch.Resonance {
		if frame == nil {
			continue
		}

		// Energy is the coherent power in the resonance layer; Flow is its
		// directional bias. A dying cascade sheds both.
		score := math.Abs(frame.Energy) + math.Abs(frame.Flow)
		resonance[frame.Symbol] = math.Max(resonance[frame.Symbol], score)
	}

	scores := make(map[string]float64, len(manifold))

	for symbol, value := range manifold {
		scores[symbol] = value + resonance[symbol]
	}

	for symbol, value := range resonance {
		if _, ok := manifold[symbol]; !ok {
			scores[symbol] = value
		}
	}

	return scores
}

/*
Continuation estimates, per symbol, the probability that price continues UP on the
next tick, in [0, 1] (0.5 = no directional edge). Unlike Momentum (which is a
magnitude), this is SIGNED — it blends the directional signals the field engine
already produces:

  - ResonanceFrame.Forecast: the supervised task head's forward-return prediction,
    the most direct next-move estimate (tanh-squashed, positive = up). Weighted
    highest because it is trained specifically to predict forward return.
  - ManifoldFrame.Momentum: the net directional push of the field.
  - CausalFrame.Uplift - Panic: the pump/dump causal read — uplift pulls up,
    panic (exhaustion / dump risk) pulls down.

The blended signed score is squashed to a probability with a logistic. It drives
the breakout decision: above a large-gain threshold, a position is held only while
continuation still favours up, and taken otherwise — and cut immediately if a held
"expect up" read is contradicted by a down tick while still in profit.
*/
func (batch Batch) Continuation() map[string]float64 {
	// Accumulate a signed directional score per symbol, then squash once.
	signed := make(map[string]float64)
	seen := make(map[string]bool)

	const (
		forecastWeight = 2.0
		momentumWeight = 1.0
		causalWeight   = 1.0
	)

	for _, frame := range batch.Resonance {
		if frame == nil {
			continue
		}

		signed[frame.Symbol] += forecastWeight * frame.Forecast
		seen[frame.Symbol] = true
	}

	for _, frame := range batch.Manifold {
		if frame == nil {
			continue
		}

		// Normalise the raw momentum into a bounded directional contribution so
		// one large-magnitude field cannot dominate the logistic.
		signed[frame.Symbol] += momentumWeight * math.Tanh(frame.Momentum)
		seen[frame.Symbol] = true
	}

	for _, frame := range batch.Causal {
		if frame == nil {
			continue
		}

		signed[frame.Symbol] += causalWeight * math.Tanh(frame.Uplift-frame.Panic)
		seen[frame.Symbol] = true
	}

	probabilities := make(map[string]float64, len(signed))

	for symbol := range seen {
		// Logistic squash of the signed score to P(up).
		probabilities[symbol] = 1.0 / (1.0 + math.Exp(-signed[symbol]))
	}

	return probabilities
}

type decisionEvaluation struct {
	evidence  decisionEvidence
	ready     bool
	stage     string
	manifold  *ManifoldFrame
	resonance *ResonanceFrame
	causal    *CausalFrame
}

type ManifoldGrid struct {
	X uint32 `json:"x"`
	Y uint32 `json:"y"`
	Z uint32 `json:"z"`
}

type ManifoldReading struct {
	PressureGradX    float64 `json:"pressureGradX"`
	PressureGradY    float64 `json:"pressureGradY"`
	PressureGradZ    float64 `json:"pressureGradZ"`
	PressureGradNorm float64 `json:"pressureGradNorm"`
	Divergence       float64 `json:"divergence"`
	CoherenceMag2    float64 `json:"coherenceMag2"`
	GuidanceSpeed    float64 `json:"guidanceSpeed"`
	ViscosityProxy   float64 `json:"viscosityProxy"`
}

type ManifoldRho struct {
	Mass     float64 `json:"mass"`
	Peak     float64 `json:"peak"`
	Entropy  float64 `json:"entropy"`
	Gradient float64 `json:"gradient"`
	CenterX  float64 `json:"centerX"`
	CenterZ  float64 `json:"centerZ"`
	SpreadX  float64 `json:"spreadX"`
	SpreadZ  float64 `json:"spreadZ"`
}

type ManifoldOscillators struct {
	Coherence float64 `json:"coherence"`
	Kinetic   float64 `json:"kinetic"`
	Thermal   float64 `json:"thermal"`
	Omega     float64 `json:"omega"`
}

type ManifoldClamp struct {
	Source    types.SourceType   `json:"source"`
	Category  types.CategoryType `json:"category"`
	Lane      int                `json:"lane"`
	PositionX float64            `json:"positionX"`
	PositionZ float64            `json:"positionZ"`
	Rho       float64            `json:"rho"`
	MomentumX float64            `json:"momentumX"`
	MomentumY float64            `json:"momentumY"`
	MomentumZ float64            `json:"momentumZ"`
	Energy    float64            `json:"energy"`
	Pressure  float64            `json:"pressure"`
}

type ManifoldCarrier struct {
	Source   types.SourceType `json:"source"`
	Role     string           `json:"role"`
	CellX    float64          `json:"cell_x"`
	CellZ    float64          `json:"cell_z"`
	Strength float64          `json:"strength"`
}

type ManifoldParticle struct {
	Source    types.SourceType `json:"source"`
	Role      string           `json:"role"`
	CellX     float64          `json:"cell_x"`
	CellY     float64          `json:"cell_y"`
	CellZ     float64          `json:"cell_z"`
	Phase     float64          `json:"phase"`
	Omega     float64          `json:"omega"`
	Amplitude float64          `json:"amplitude"`
	Heat      float64          `json:"heat"`
	VelX      float64          `json:"vel_x"`
	VelY      float64          `json:"vel_y"`
	VelZ      float64          `json:"vel_z"`
	Speed     float64          `json:"speed"`
}

type ManifoldFrame struct {
	Source      types.SourceType    `json:"source"`
	Symbol      string              `json:"symbol"`
	At          string              `json:"at"`
	Price       decimal.Decimal     `json:"price"`
	Momentum    float64             `json:"momentum"`
	Pressure    float64             `json:"pressure"`
	Category    types.CategoryType  `json:"category"`
	Strength    float64             `json:"strength"`
	Shock       float64             `json:"shock"`
	Resistance  float64             `json:"resistance"`
	Peak        float64             `json:"peak"`
	Grid        ManifoldGrid        `json:"grid"`
	Rho         [][]float64         `json:"rho"`
	Summary     ManifoldRho         `json:"summary"`
	Reading     ManifoldReading     `json:"reading"`
	Projection  ManifoldReading     `json:"projection"`
	Oscillators ManifoldOscillators `json:"oscillators"`
	Clamps      []ManifoldClamp     `json:"clamps"`
	Carriers    []ManifoldCarrier   `json:"carriers"`
	Particles   []ManifoldParticle  `json:"particles"`
}

type ResonanceFrame struct {
	Source     types.SourceType   `json:"source"`
	Symbol     string             `json:"symbol"`
	At         string             `json:"at"`
	Category   types.CategoryType `json:"category"`
	Confidence float64            `json:"confidence"`
	Flow       float64            `json:"flow"`
	Stress     float64            `json:"stress"`
	Coupling   float64            `json:"coupling"`
	Baseline   float64            `json:"baseline"`
	Energy     float64            `json:"energy"`
	Surprise   float64            `json:"surprise"`
	Forecast   float64            `json:"forecast"`
	Latent     []float64          `json:"latent"`
}

type CausalFrame struct {
	Source       types.SourceType   `json:"source"`
	Symbol       string             `json:"symbol"`
	At           string             `json:"at"`
	Category     types.CategoryType `json:"category"`
	Confidence   float64            `json:"confidence"`
	Strength     float64            `json:"strength"`
	Baseline     float64            `json:"baseline"`
	Uplift       float64            `json:"uplift"`
	Intervention float64            `json:"intervention"`
	Beta         float64            `json:"beta"`
	Panic        float64            `json:"panic"`
	Residual     float64            `json:"residual"`
}

/*
minimalManifold builds a lightweight manifold frame carrying only the momentum
and pressure the boundary clamps already express, without a physical settle. It
is emitted on the price_zero early-bail (the dominant no-frame stage) so a HELD
position still contributes a live momentum score to Batch.Momentum() rather than
dropping out and stalling its peakMomentum. Batch.Momentum() takes the max per
symbol, so this fallback never shadows a full frame from another source in the
same cycle — it only fills a genuine gap.
*/
func (frame boundaryFrame) minimalManifold() *ManifoldFrame {
	return &ManifoldFrame{
		Source: types.SourceManifold,
		Symbol: frame.symbol,
		At:     frame.at(),
		Price:  frame.price,
		// PhysicalField is the neutral manifold category. A frame must carry a
		// valid category or the cortex topology rejects it (empty category is an
		// error), which would fail cortex.Measure for the whole tick.
		Category: types.PhysicalField,
		Momentum: frame.netMomentum(),
		Pressure: frame.netPressure(),
	}
}

func (frame boundaryFrame) manifold(
	config ManifoldGrid,
	physical physicalEvidence,
) *ManifoldFrame {
	clamps := make([]ManifoldClamp, 0, len(frame.clamps))

	for _, clamp := range frame.clamps {
		clamps = append(clamps, ManifoldClamp{
			Source:    clamp.source,
			Category:  clamp.category,
			Lane:      clamp.lane,
			PositionX: clamp.positionX,
			PositionZ: clamp.positionZ,
			Rho:       clamp.rho,
			MomentumX: clamp.momX,
			MomentumY: clamp.momY,
			MomentumZ: clamp.momZ,
			Energy:    clamp.energy,
			Pressure:  clamp.pressure,
		})
	}

	particles := make([]ManifoldParticle, 0, len(physical.particles))
	carriers := make([]ManifoldCarrier, 0, len(physical.particles))

	for index, particle := range physical.particles {
		clamp := frame.clamps[index]
		role := string(clamp.category)
		speed := math.Sqrt(
			particle.VelX*particle.VelX +
				particle.VelY*particle.VelY +
				particle.VelZ*particle.VelZ,
		)

		particles = append(particles, ManifoldParticle{
			Source:    clamp.source,
			Role:      role,
			CellX:     particle.PosX,
			CellY:     particle.PosY,
			CellZ:     particle.PosZ,
			Phase:     particle.Phase,
			Omega:     particle.Omega,
			Amplitude: particle.Amplitude,
			Heat:      particle.Heat,
			VelX:      particle.VelX,
			VelY:      particle.VelY,
			VelZ:      particle.VelZ,
			Speed:     speed,
		})

		carriers = append(carriers, ManifoldCarrier{
			Source:   clamp.source,
			Role:     role,
			CellX:    particle.PosX,
			CellZ:    particle.PosZ,
			Strength: math.Abs(particle.Amplitude),
		})
	}

	return &ManifoldFrame{
		Source:     types.SourceManifold,
		Symbol:     frame.symbol,
		At:         frame.at(),
		Price:      frame.price,
		Momentum:   frame.netMomentum(),
		Pressure:   frame.netPressure(),
		Category:   physical.category,
		Strength:   physical.strength,
		Shock:      physical.shock,
		Resistance: physical.resistance,
		Peak:       physical.rho.peak,
		Grid:       config,
		Rho:        physical.rhoRows,
		Summary: ManifoldRho{
			Mass:     physical.rho.mass,
			Peak:     physical.rho.peak,
			Entropy:  physical.rho.entropy,
			Gradient: physical.rho.gradient,
			CenterX:  physical.rho.centerX,
			CenterZ:  physical.rho.centerZ,
			SpreadX:  physical.rho.spreadX,
			SpreadZ:  physical.rho.spreadZ,
		},
		Reading:     readingFrame(physical.reading),
		Projection:  readingFrame(physical.projection),
		Oscillators: oscillatorFrame(physical.oscillators),
		Clamps:      clamps,
		Carriers:    carriers,
		Particles:   particles,
	}
}

func (frame boundaryFrame) resonance(
	predictive predictiveEvidence,
) *ResonanceFrame {
	return &ResonanceFrame{
		Source:     types.SourceResonance,
		Symbol:     frame.symbol,
		At:         frame.at(),
		Category:   predictive.category,
		Confidence: predictive.confidence,
		Flow:       predictive.flow,
		Stress:     predictive.stress,
		Coupling:   predictive.coupling,
		Baseline:   predictive.baseline,
		Energy:     predictive.energy,
		Surprise:   predictive.surprise,
		Forecast:   predictive.forecast,
		Latent:     append([]float64(nil), predictive.latent...),
	}
}

func (frame boundaryFrame) causal(
	counterfactual counterfactualEvidence,
) *CausalFrame {
	return &CausalFrame{
		Source:       types.SourceCausal,
		Symbol:       frame.symbol,
		At:           frame.at(),
		Category:     counterfactual.category,
		Confidence:   counterfactual.confidence,
		Strength:     counterfactual.strength,
		Baseline:     counterfactual.baseline,
		Uplift:       counterfactual.uplift,
		Intervention: counterfactual.intervention,
		Beta:         counterfactual.beta,
		Panic:        counterfactual.panic,
		Residual:     counterfactual.residual,
	}
}

func (frame boundaryFrame) at() string {
	if frame.eventAt.IsZero() {
		return ""
	}

	return frame.eventAt.UTC().Format(time.RFC3339Nano)
}

func readingFrame(reading pmanifold.Reading) ManifoldReading {
	return ManifoldReading{
		PressureGradX:    reading.PressureGradX,
		PressureGradY:    reading.PressureGradY,
		PressureGradZ:    reading.PressureGradZ,
		PressureGradNorm: reading.PressureGradNorm,
		Divergence:       reading.Divergence,
		CoherenceMag2:    reading.CoherenceMag2,
		GuidanceSpeed:    reading.GuidanceSpeed,
		ViscosityProxy:   reading.ViscosityProxy,
	}
}

func oscillatorFrame(oscillators oscillatorEvidence) ManifoldOscillators {
	return ManifoldOscillators{
		Coherence: oscillators.coherence,
		Kinetic:   oscillators.kinetic,
		Thermal:   oscillators.thermal,
		Omega:     oscillators.omega,
	}
}
