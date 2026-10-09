package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Peer is one neighbour's correlation, overlap support, and optional energy rate.
*/
type Peer struct {
	Correlation float64
	Support     float64
	PeerEnergy  float64
}

/*
CohortSummary is one delivery's admitted-peer reductions.
*/
type CohortSummary struct {
	PeersSeen           float64
	Peers               float64
	RejectedPeers       float64
	TotalSupport        float64
	EffectivePeers      float64
	SignedCorrelation   float64
	AbsoluteCorrelation float64
	PeerEnergyRate      float64
	Dispersion          float64
	Defined             bool
	FisherDefined       bool
}

/*
Cohort summarizes one peer run. Support below two is excluded.
*/
type Cohort struct {
	err error
	out CohortSummary
}

func NewCohort() core.Primitive {
	return &Cohort{}
}

func (op *Cohort) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var admitted []Peer
		seen := 0.0

		for arriving := range in {
			seen++
			peer := *(*Peer)(arriving)

			if peer.Support >= 2 {
				admitted = append(admitted, peer)
			}
		}

		summary := CohortSummary{
			PeersSeen:     seen,
			Peers:         float64(len(admitted)),
			RejectedPeers: seen - float64(len(admitted)),
		}

		if len(admitted) == 0 {
			op.out = summary

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

		totalFisherWeight := 0.0

		for _, item := range admitted {
			weight := item.Support
			totalWeight += weight
			sumWeightSq += weight * weight
			sumSigned += weight * item.Correlation
			sumAbsolute += weight * math.Abs(item.Correlation)
			sumEnergy += weight * item.PeerEnergy

			if math.Abs(item.Correlation) < 1.0 {
				zValue := math.Atanh(item.Correlation)
				sumZ += weight * zValue
				sumZ2 += weight * zValue * zValue
				totalFisherWeight += weight
			}
		}

		signedMean := sumSigned / totalWeight
		absoluteMean := sumAbsolute / totalWeight
		energyMean := sumEnergy / totalWeight
		kish := (totalWeight * totalWeight) / sumWeightSq

		dispersion := 0.0
		fisherDefined := false

		if totalFisherWeight > 0 {
			zMean := sumZ / totalFisherWeight
			weightedVariance := (sumZ2 / totalFisherWeight) - (zMean * zMean)

			if weightedVariance >= 0 {
				dispersion = math.Sqrt(weightedVariance)
				fisherDefined = true
			}
		}

		summary.TotalSupport = totalWeight
		summary.EffectivePeers = kish
		summary.SignedCorrelation = signedMean
		summary.AbsoluteCorrelation = absoluteMean
		summary.PeerEnergyRate = energyMean
		summary.Dispersion = dispersion
		summary.Defined = totalWeight > 0
		summary.FisherDefined = fisherDefined

		op.out = summary

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}

func (op *Cohort) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
