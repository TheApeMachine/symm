package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Cohort summarizes one peer run. Each peer arrival is
[3]float64{correlation, support, peerEnergy}. Support below two is excluded.
It yields one
[11]float64{peersSeen, peers, rejected, totalSupport, effectivePeers,
signedCorrelation, absoluteCorrelation, peerEnergyRate, dispersion,
defined, fisherDefined}.
*/
type Cohort struct {
	*core.PrimitiveError
	out [11]float64
}

func NewCohort() core.Primitive {
	return &Cohort{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Cohort) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var admitted [][3]float64
		seen := 0.0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			seen++
			peer := *(*[3]float64)(arriving)

			if peer[1] >= 2 {
				admitted = append(admitted, peer)
			}
		}

		op.out = [11]float64{
			seen,
			float64(len(admitted)),
			seen - float64(len(admitted)),
		}

		if len(admitted) == 0 {
			if !yield(unsafe.Pointer(&op.out)) {
				return
			}

			return
		}

		totalWeight := 0.0
		sumWeightSq := 0.0
		sumSigned := 0.0
		sumAbsolute := 0.0
		sumEnergy := 0.0
		sumZ := 0.0
		sumZ2 := 0.0

		for _, p := range admitted {
			w := p[1]
			totalWeight += w
			sumWeightSq += w * w
			sumSigned += w * p[0]
			sumAbsolute += w * math.Abs(p[0])
			sumEnergy += w * p[2]
			z := math.Atanh(p[0])
			sumZ += w * z
			sumZ2 += w * z * z
		}

		signedMean := sumSigned / totalWeight
		absoluteMean := sumAbsolute / totalWeight
		energyMean := sumEnergy / totalWeight
		kish := (totalWeight * totalWeight) / sumWeightSq

		zMean := sumZ / totalWeight
		weightedVariance := (sumZ2 / totalWeight) - (zMean * zMean)
		dispersion := 0.0
		fisherDefined := 0.0

		if weightedVariance >= 0 {
			dispersion = math.Sqrt(weightedVariance)
			fisherDefined = 1
		}

		defined := 0.0

		if totalWeight > 0 {
			defined = 1
		}

		op.out = [11]float64{
			seen,
			float64(len(admitted)),
			seen - float64(len(admitted)),
			totalWeight,
			kish,
			signedMean,
			absoluteMean,
			energyMean,
			dispersion,
			defined,
			fisherDefined,
		}

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}
