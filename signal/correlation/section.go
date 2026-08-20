package correlation

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/nomagique/statistic"
)

const correlationSupportFloor = 2

type priceSample struct {
	at    time.Time
	value float64
}

type symbolState struct {
	samples       []priceSample
	observedCount int
	energy        float64
}

type Scores struct {
	Correlation    float64
	Signed         float64
	RelativeEnergy float64
	Herd           float64
	Alpha          float64
	Noise          float64
	Stress         float64
	SNR            float64
	ObservedFrom   time.Time
}

type Section struct {
	symbols map[string]*symbolState
}

func NewSection() *Section {
	return &Section{symbols: make(map[string]*symbolState)}
}

func (section *Section) Observe(symbol string, price float64, at time.Time) bool {
	if symbol == "" || price <= 0 || at.IsZero() {
		return false
	}

	state := section.symbols[symbol]

	if state == nil {
		state = &symbolState{}
		section.symbols[symbol] = state
	}

	if len(state.samples) > 0 && !at.After(state.samples[len(state.samples)-1].at) {
		return false
	}

	state.observedCount++
	state.samples = append(state.samples, priceSample{at: at, value: price})
	retention := correlationRetention(state.observedCount)

	if len(state.samples) > retention {
		state.samples = state.samples[len(state.samples)-retention:]
	}

	state.energy = pathEnergy(state.samples)

	return true
}

func (section *Section) Scores(symbol string) (Scores, bool) {
	state := section.symbols[symbol]

	if state == nil || state.energy <= 0 {
		return Scores{}, false
	}

	totalSupport := 0.0
	weightedSigned := 0.0
	weightedAbsolute := 0.0
	weightedPeerEnergy := 0.0

	for peerSymbol, peer := range section.symbols {
		if peerSymbol == symbol || peer == nil || peer.energy <= 0 {
			continue
		}

		correlation, support, ready := supportedCorrelation(state.samples, peer.samples)

		if !ready || support < correlationSupportFloor {
			continue
		}

		weight := float64(support)
		totalSupport += weight
		weightedSigned += correlation * weight
		weightedAbsolute += math.Abs(correlation) * weight
		weightedPeerEnergy += peer.energy * weight
	}

	if totalSupport == 0 || weightedPeerEnergy <= 0 {
		return Scores{}, false
	}

	signed := weightedSigned / totalSupport
	cohortCorrelation := weightedAbsolute / totalSupport
	peerEnergy := weightedPeerEnergy / totalSupport
	relativeEnergy := state.energy / peerEnergy
	excessEnergy := max(0, relativeEnergy-1)
	energyDeficit := max(0, 1-relativeEnergy)
	excessMass := excessEnergy / (1 + excessEnergy)
	herd := max(0, signed) / (1 + excessEnergy)
	alpha := excessMass / (1 + max(0, signed))
	noise := max(0, 1-cohortCorrelation) / (1 + excessEnergy + energyDeficit)
	stress := max(0, -signed)
	snr := hypothesisSeparation(herd, alpha, noise, stress)

	return Scores{
		Correlation: cohortCorrelation, Signed: signed, RelativeEnergy: relativeEnergy,
		Herd: herd, Alpha: alpha, Noise: noise, Stress: stress, SNR: snr,
		ObservedFrom: state.samples[0].at,
	}, true
}

func supportedCorrelation(left, right []priceSample) (float64, int, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, 0, false
	}

	leftUsed := make([]bool, len(left)-1)
	rightUsed := make([]bool, len(right)-1)
	covariance := 0.0
	support := 0

	for leftIndex := 0; leftIndex < len(left)-1; leftIndex++ {
		leftReturn := math.Log(left[leftIndex+1].value / left[leftIndex].value)

		for rightIndex := 0; rightIndex < len(right)-1; rightIndex++ {
			if !intervalsOverlap(
				left[leftIndex].at, left[leftIndex+1].at,
				right[rightIndex].at, right[rightIndex+1].at,
			) {
				continue
			}

			rightReturn := math.Log(right[rightIndex+1].value / right[rightIndex].value)
			covariance += leftReturn * rightReturn
			leftUsed[leftIndex] = true
			rightUsed[rightIndex] = true
			support++
		}
	}

	leftVariance := supportedVariance(left, leftUsed)
	rightVariance := supportedVariance(right, rightUsed)

	if support == 0 || leftVariance <= 0 || rightVariance <= 0 {
		return 0, support, false
	}

	value := covariance / math.Sqrt(leftVariance*rightVariance)

	return max(-1, min(1, value)), support, true
}

func supportedVariance(samples []priceSample, used []bool) float64 {
	variance := 0.0

	for index, included := range used {
		if !included {
			continue
		}

		value := math.Log(samples[index+1].value / samples[index].value)
		variance += value * value
	}

	return variance
}

func intervalsOverlap(leftFrom, leftTo, rightFrom, rightTo time.Time) bool {
	return leftFrom.Before(rightTo) && rightFrom.Before(leftTo)
}

func pathEnergy(samples []priceSample) float64 {
	returns := make([]float64, 0, len(samples)-1)

	for index := 1; index < len(samples); index++ {
		seconds := samples[index].at.Sub(samples[index-1].at).Seconds()

		if seconds <= 0 {
			continue
		}

		returns = append(returns, math.Abs(
			math.Log(samples[index].value/samples[index-1].value),
		)/math.Sqrt(seconds))
	}

	median, ready := statistic.MedianOf(returns)

	if !ready {
		return 0
	}

	return median
}

func correlationRetention(observedCount int) int {
	if observedCount <= 1 {
		return observedCount
	}

	shortWindow := int(math.Ceil(math.Sqrt(float64(observedCount))))
	return min(observedCount, max(shortWindow+1, shortWindow*2))
}

func hypothesisSeparation(scores ...float64) float64 {
	sort.Sort(sort.Reverse(sort.Float64Slice(scores)))

	if len(scores) < 2 || scores[0] <= 0 {
		return 0
	}

	return (scores[0] - scores[1]) / scores[0]
}
