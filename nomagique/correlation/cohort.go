package correlation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
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
CohortSummary is one delivery's admitted-peer reductions. A new run is a new
cohort; no Reset method or replay convention exists.
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
Cohort summarizes one peer run. Support below two is excluded, with counts
exposed. The caller supplies the dispersion transform, applied once per
admitted peer.
*/
type Cohort struct {
	core.Base[Peer, CohortSummary]
	transform *calculus.Atanh[float64]
	finite    *logic.Finite[float64]
}

func NewCohort(transform *calculus.Atanh[float64]) *Cohort {
	return &Cohort{transform: transform, finite: logic.NewFinite[float64]()}
}

func (op *Cohort) Next(
	in iter.Seq[core.Primitive[Peer, Peer]],
) iter.Seq[core.Primitive[CohortSummary, CohortSummary]] {
	return func(yield func(core.Primitive[CohortSummary, CohortSummary]) bool) {
		var admitted []Peer
		seen := 0.0

		for arriving := range in {
			seen++
			peer := arriving.Read()
			finite, err := transport.Evaluate(op.finite, transport.Values(peer.Correlation))

			if err != nil {
				op.Error(err)
				return
			}

			if finite {
				support, err := transport.Evaluate(op.finite, transport.Values(peer.Support))

				if err != nil {
					op.Error(err)
					return
				}

				if support && peer.Support >= 2 {
					admitted = append(admitted, peer)
				}
			}
		}

		summary := CohortSummary{
			PeersSeen:           seen,
			Peers:               float64(len(admitted)),
			RejectedPeers:       seen - float64(len(admitted)),
			SignedCorrelation:   math.NaN(),
			AbsoluteCorrelation: math.NaN(),
			PeerEnergyRate:      math.NaN(),
			Dispersion:          math.NaN(),
			EffectivePeers:      math.NaN(),
		}

		if len(admitted) == 0 {
			if !yield(op.Carrier(summary)) {
				return
			}

			return
		}

		weights := make([]float64, len(admitted))
		signed := make([]equation.Weighted[float64], len(admitted))
		absolute := make([]equation.Weighted[float64], len(admitted))
		energy := make([]equation.Weighted[float64], len(admitted))
		transformed := make([]equation.Weighted[float64], len(admitted))

		for index, peer := range admitted {
			z, err := transport.Evaluate(op.transform, transport.Values(peer.Correlation))

			if err != nil {
				op.Error(err)
				return
			}

			weights[index] = peer.Support
			signed[index] = equation.Weighted[float64]{Weight: peer.Support, Value: peer.Correlation}
			absolute[index] = equation.Weighted[float64]{Weight: peer.Support, Value: math.Abs(peer.Correlation)}
			energy[index] = equation.Weighted[float64]{Weight: peer.Support, Value: peer.PeerEnergy}
			transformed[index] = equation.Weighted[float64]{Weight: peer.Support, Value: z}
		}

		kish := last(equation.NewKish[float64]().Next(transport.Values(weights...)))
		signedMean := last(equation.NewWeightedMean[float64]().Next(transport.Values(signed...)))
		absoluteMean := last(equation.NewWeightedMean[float64]().Next(transport.Values(absolute...)))
		energyMean := last(equation.NewWeightedMean[float64]().Next(transport.Values(energy...)))
		variance := last(equation.NewWeightedVariance[float64]().Next(transport.Values(transformed...)))
		dispersion, err := transport.Evaluate(calculus.NewSqrt[float64](), transport.Values(variance))

		if err != nil {
			op.Error(err)
			return
		}

		total := 0.0

		for _, weight := range weights {
			total += weight
		}

		finite, err := transport.Evaluate(op.finite, transport.Values(dispersion))

		if err != nil {
			op.Error(err)
			return
		}

		summary.TotalSupport = total
		summary.EffectivePeers = kish
		summary.SignedCorrelation = signedMean
		summary.AbsoluteCorrelation = absoluteMean
		summary.PeerEnergyRate = energyMean
		summary.Dispersion = dispersion
		summary.Defined = total > 0
		summary.FisherDefined = finite

		if !yield(op.Carrier(summary)) {
			return
		}
	}
}

func last[U any](seq iter.Seq[core.Primitive[U, U]]) U {
	var value U

	for arriving := range seq {
		value = arriving.Read()
	}

	return value
}
