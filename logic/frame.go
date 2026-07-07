package logic

import (
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

type Batch struct {
	Actions   []*Action
	Manifold  []*ManifoldFrame
	Resonance []*ResonanceFrame
	Causal    []*CausalFrame
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

type ManifoldFrame struct {
	Source      types.SourceType    `json:"source"`
	Symbol      string              `json:"symbol"`
	At          string              `json:"at"`
	Price       float64             `json:"price"`
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

func (frame boundaryFrame) manifold(
	config ManifoldGrid,
	physical physicalEvidence,
) *ManifoldFrame {
	clamps := make([]ManifoldClamp, 0, len(frame.clamps))
	carriers := make([]ManifoldCarrier, 0, len(frame.clamps))

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
		carriers = append(carriers, ManifoldCarrier{
			Source:   clamp.source,
			Role:     string(clamp.category),
			CellX:    clamp.positionX * float64(max(config.X, 1)-1),
			CellZ:    clamp.positionZ * float64(max(config.Z, 1)-1),
			Strength: clamp.rho,
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
