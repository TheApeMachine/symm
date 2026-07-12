package manifold

import (
	"math"
	"sort"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
ModeExtractor derives order-flow oscillator modes from carrier cohorts.
The population may be empty when no coherent mode is present.
*/
type ModeExtractor struct {
	config *pmanifold.Config
}

func NewModeExtractor(config *pmanifold.Config) *ModeExtractor {
	return &ModeExtractor{config: config}
}

func (extractor *ModeExtractor) Modes(cohorts []Cohort, eventDeltaT float64) []pmanifold.Oscillator {
	if len(cohorts) == 0 || eventDeltaT <= 0 {
		return nil
	}

	candidates := make([]modeCandidate, 0, len(cohorts))
	totalMass := 0.0

	for _, cohort := range cohorts {
		totalMass += cohort.Mass
		candidate, ok := extractor.candidateFromCohort(cohort, eventDeltaT)

		if !ok {
			continue
		}

		candidates = append(candidates, candidate)
	}

	if len(candidates) == 0 || totalMass <= 0 {
		return nil
	}

	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].coherenceEnergy > candidates[right].coherenceEnergy
	})

	limit := int(extractor.config.MaxModes)

	if limit <= 0 {
		limit = len(candidates)
	}

	if len(candidates) < limit {
		limit = len(candidates)
	}

	modes := make([]pmanifold.Oscillator, 0, limit)

	for index := 0; index < limit; index++ {
		oscillator := candidates[index].oscillator
		oscillator.Amplitude /= totalMass
		modes = append(modes, oscillator)
	}

	return modes
}

/*
SpectrumAnchor derives one event-rate oscillator when no coherent cohort motion exists.
*/
func (extractor *ModeExtractor) SpectrumAnchor(cohorts []Cohort, eventDeltaT float64) []pmanifold.Oscillator {
	if len(cohorts) == 0 || eventDeltaT <= 0 {
		return nil
	}

	dominant := cohorts[0]
	totalMass := 0.0
	flowBias := 0.0

	for _, cohort := range cohorts {
		totalMass += cohort.Mass

		sideSign := 1.0

		if cohort.Side == OrderSideBid {
			sideSign = -1.0
		}

		flowBias += sideSign * cohort.Mass
	}

	if totalMass <= 0 {
		return nil
	}

	for _, cohort := range cohorts {
		if cohort.Mass > dominant.Mass {
			dominant = cohort
		}
	}

	omega := extractor.eventSpectrumOmega(eventDeltaT)
	phase := flowBias / totalMass
	heatBudget := (dominant.SecondMoment.Price + dominant.SecondMoment.Size + dominant.SecondMoment.Age) / 3

	return []pmanifold.Oscillator{{
		Phase:     phase,
		Omega:     omega,
		Amplitude: dominant.Mass / totalMass,
		PosX:      extractor.centeredPosition(dominant.Centroid.Price, extractor.config.DomainX),
		PosY:      extractor.centeredPosition(dominant.Centroid.Size, extractor.config.DomainY),
		PosZ:      extractor.unitPosition(dominant.Centroid.Age, extractor.config.DomainZ),
		Heat:      heatBudget,
	}}
}

type modeCandidate struct {
	coherenceEnergy float64
	oscillator      pmanifold.Oscillator
}

func (extractor *ModeExtractor) candidateFromCohort(
	cohort Cohort,
	eventDeltaT float64,
) (modeCandidate, bool) {
	if cohort.Mass <= 0 {
		return modeCandidate{}, false
	}

	speed := math.Hypot(
		cohort.Velocity.Price,
		math.Hypot(cohort.Velocity.Size, cohort.Velocity.Age),
	)

	if speed <= 0 {
		return modeCandidate{}, false
	}

	omega := extractor.eventSpectrumOmega(eventDeltaT) * speed
	omega = extractor.clampOmega(omega)

	if omega <= 0 {
		return modeCandidate{}, false
	}

	phase := math.Atan2(cohort.Velocity.Age, cohort.Velocity.Price)
	heatBudget := (cohort.SecondMoment.Price + cohort.SecondMoment.Size + cohort.SecondMoment.Age) / 3

	return modeCandidate{
		coherenceEnergy: cohort.Mass * speed,
		oscillator: pmanifold.Oscillator{
			Phase:     phase,
			Omega:     omega,
			Amplitude: cohort.Mass,
			PosX:      extractor.centeredPosition(cohort.Centroid.Price, extractor.config.DomainX),
			PosY:      extractor.centeredPosition(cohort.Centroid.Size, extractor.config.DomainY),
			PosZ:      extractor.unitPosition(cohort.Centroid.Age, extractor.config.DomainZ),
			Heat:      heatBudget,
			VelX:      cohort.Velocity.Price,
			VelY:      cohort.Velocity.Size,
			VelZ:      cohort.Velocity.Age,
		},
	}, true
}

func (extractor *ModeExtractor) eventSpectrumOmega(eventDeltaT float64) float64 {
	return 2 * math.Pi / eventDeltaT
}

func (extractor *ModeExtractor) clampOmega(omega float64) float64 {
	minOmega := extractor.config.GateWidthMin()
	maxOmega := extractor.config.GateWidthMax()

	if omega < minOmega {
		return minOmega
	}

	if omega > maxOmega {
		return maxOmega
	}

	return omega
}

func (extractor *ModeExtractor) centeredPosition(coordinate float64, domain float64) float64 {
	if domain <= 0 {
		return 0
	}

	position := coordinate + domain/2

	if position < 0 {
		return 0
	}

	if position >= domain {
		return math.Nextafter(domain, 0)
	}

	return position
}

func (extractor *ModeExtractor) unitPosition(coordinate float64, domain float64) float64 {
	if domain <= 0 {
		return 0
	}

	position := min(max(coordinate, 0), 1) * domain

	if position >= domain {
		return math.Nextafter(domain, 0)
	}

	return position
}
