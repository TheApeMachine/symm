package logic

import (
	"hash/fnv"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/geometry"
)

/*
EigenmodeName identifies an orthogonal microstructure factor family.
Correlated sources (CVD, pumpdump, depthflow, exhaustion) collapse into
momentum instead of requiring simultaneous boolean matches.
*/
type EigenmodeName string

const (
	EigenmodeMomentum  EigenmodeName = "momentum"
	EigenmodeStructure EigenmodeName = "structure"
	EigenmodeRisk      EigenmodeName = "risk"
	EigenmodeBreadth   EigenmodeName = "breadth"
)

/*
EigenmodeRef selects a mode score inside playbook conditions.
*/
type EigenmodeRef struct {
	Mode EigenmodeName `yaml:"mode" json:"mode"`
}

var eigenmodeFamilies = map[SourceType]EigenmodeName{
	SourceCVD:         EigenmodeMomentum,
	SourcePumpDump:    EigenmodeMomentum,
	SourceDepthFlow:   EigenmodeMomentum,
	SourceExhaustion:  EigenmodeMomentum,
	SourceFluid:       EigenmodeStructure,
	SourceManifold:    EigenmodeStructure,
	SourceCorrelation: EigenmodeStructure,
	SourceHawkes:      EigenmodeStructure,
	SourceCausal:      EigenmodeRisk,
	SourceToxicity:    EigenmodeRisk,
	SourceLiquidity:   EigenmodeRisk,
	SourceLeadLag:     EigenmodeBreadth,
	SourceSentiment:   EigenmodeBreadth,
}

var sourceOriginToSource map[uint64]SourceType

func init() {
	sourceOriginToSource = make(map[uint64]SourceType, len(eigenmodeFamilies))

	for source := range eigenmodeFamilies {
		sourceOriginToSource[sourceOriginID(source)] = source
	}
}

/*
BuildEigenmodeScores partitions live measurements into orthogonal modes.
The score is normalized cluster energy in 0..1.
*/
func BuildEigenmodeScores(measurements []*datura.Artifact) map[EigenmodeName]float64 {
	participants := make([]geometry.ModeParticipant, 0, len(measurements))
	energyByMode := make(map[EigenmodeName]float64, 4)

	for _, measurement := range measurements {
		origin, err := measurement.Origin()

		if err != nil {
			continue
		}

		source := SourceType(origin)

		if source == SourceNone {
			continue
		}

		strength := datura.Peek[float64](measurement, "output", "strength")

		if strength <= 0 {
			continue
		}

		mode, ok := eigenmodeFamilies[source]

		if !ok {
			continue
		}

		confidence := datura.Peek[float64](measurement, "output", "confidence")
		energy := confidence * strength

		if energy <= 0 {
			continue
		}

		participants = append(participants, geometry.ModeParticipant{
			Origin: sourceOriginID(source),
			Energy: energy,
		})
		energyByMode[mode] += energy
	}

	if len(participants) == 0 {
		return map[EigenmodeName]float64{}
	}

	modes, dominantIndex := geometry.DetectModes(
		participants,
		eigenmodeCouplingThreshold(),
		eigenmodeCoupling,
	)

	scores := make(map[EigenmodeName]float64, len(energyByMode))
	totalEnergy := 0.0

	for _, energy := range energyByMode {
		totalEnergy += energy
	}

	if totalEnergy <= 0 {
		return scores
	}

	for mode, energy := range energyByMode {
		scores[mode] = energy / totalEnergy
	}

	if dominantIndex >= 0 && dominantIndex < len(modes) {
		dominantEnergy := modes[dominantIndex].Energy()

		if dominantEnergy > 0 {
			for mode, energy := range energyByMode {
				scores[mode] = math.Max(scores[mode], dominantEnergyEnergyRatio(
					energy,
					dominantEnergy,
					totalEnergy,
				))
			}
		}
	}

	return scores
}

func eigenmodeCouplingThreshold() float64 {
	maxCoupling := 0.0

	for sourceA := range eigenmodeFamilies {
		for sourceB := range eigenmodeFamilies {
			coupling := eigenmodeCoupling(
				sourceOriginID(sourceA),
				sourceOriginID(sourceB),
			)

			if coupling > maxCoupling {
				maxCoupling = coupling
			}
		}
	}

	return maxCoupling
}

func dominantEnergyEnergyRatio(localEnergy, dominantEnergy, totalEnergy float64) float64 {
	if totalEnergy <= 0 {
		return 0
	}

	coherence := localEnergy / totalEnergy
	dominance := dominantEnergy / totalEnergy

	return math.Max(coherence, dominance)
}

func eigenmodeCoupling(originA, originB uint64) float64 {
	sourceA := sourceFromOrigin(originA)
	sourceB := sourceFromOrigin(originB)

	if sourceA == SourceNone || sourceB == SourceNone {
		return 0
	}

	modeA, okA := eigenmodeFamilies[sourceA]
	modeB, okB := eigenmodeFamilies[sourceB]

	if !okA || !okB {
		return 0
	}

	if modeA != modeB {
		return 0
	}

	return 1
}

func sourceOriginID(source SourceType) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(source))

	return hasher.Sum64()
}

func sourceFromOrigin(origin uint64) SourceType {
	source, ok := sourceOriginToSource[origin]

	if !ok {
		return SourceNone
	}

	return source
}

/*
EigenmodeScore returns the normalized score for one mode.
*/
func EigenmodeScore(measurements []*datura.Artifact, mode EigenmodeName) (float64, bool) {
	scores := BuildEigenmodeScores(measurements)
	score, ok := scores[mode]

	if !ok || score <= 0 {
		return 0, false
	}

	return score, true
}
