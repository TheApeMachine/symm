package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Peer is one neighbour's covariance score, overlap support, and optional energy
rate.
*/
type Peer struct {
	Score      float64
	Support    float64
	PeerEnergy float64
}

/*
CohortSummary is one delivery's admitted-peer reductions.
*/
type CohortSummary struct {
	PeersSeen         float64
	Peers             float64
	RejectedPeers     float64
	TotalSupport      float64
	EffectivePeers    float64
	SignedScore       float64
	AbsoluteScore     float64
	PeerEnergyRate    float64
	Dispersion        float64
	Defined           bool
	DispersionDefined bool
}

/*
Cohort summarizes one peer run with support-weighted means of the peers'
covariance scores. Support below two is excluded. Dispersion is the
support-weighted standard deviation of the scores, defined from two admitted
peers.
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

		for _, item := range admitted {
			weight := item.Support
			totalWeight += weight
			sumWeightSq += weight * weight
			sumSigned += weight * item.Score
			sumAbsolute += weight * math.Abs(item.Score)
			sumEnergy += weight * item.PeerEnergy
		}

		signedMean := sumSigned / totalWeight
		absoluteMean := sumAbsolute / totalWeight
		energyMean := sumEnergy / totalWeight
		kish := (totalWeight * totalWeight) / sumWeightSq

		dispersion := 0.0
		dispersionDefined := false

		if len(admitted) > 1 {
			squared := 0.0

			for _, item := range admitted {
				deviation := item.Score - signedMean
				squared += item.Support * deviation * deviation
			}

			dispersion = math.Sqrt(squared / totalWeight)
			dispersionDefined = true
		}

		summary.TotalSupport = totalWeight
		summary.EffectivePeers = kish
		summary.SignedScore = signedMean
		summary.AbsoluteScore = absoluteMean
		summary.PeerEnergyRate = energyMean
		summary.Dispersion = dispersion
		summary.Defined = totalWeight > 0
		summary.DispersionDefined = dispersionDefined

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
