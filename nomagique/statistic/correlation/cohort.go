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
	*core.PrimitiveError
}

func NewCohort() *Cohort {
	return &Cohort{PrimitiveError: core.NewPrimitiveError()}
}

func (cohort *Cohort) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var admitted []Peer
		seen := 0.0

		for arriving := range in {
			seen++
			peer := *(*Peer)(arriving)

			if peer.Support >= 2 && peer.Correlation >= -1 && peer.Correlation <= 1 {
				admitted = append(admitted, peer)
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
			if !yield(unsafe.Pointer(&summary)) {
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
			w := p.Support
			totalWeight += w
			sumWeightSq += w * w
			sumSigned += w * p.Correlation
			sumAbsolute += w * math.Abs(p.Correlation)
			sumEnergy += w * p.PeerEnergy
			z := math.Atanh(p.Correlation)
			sumZ += w * z
			sumZ2 += w * z * z
		}

		signedMean := sumSigned / totalWeight
		absoluteMean := sumAbsolute / totalWeight
		energyMean := sumEnergy / totalWeight
		kish := (totalWeight * totalWeight) / sumWeightSq

		zMean := sumZ / totalWeight
		weightedVariance := (sumZ2 / totalWeight) - (zMean * zMean)
		dispersion := math.Sqrt(weightedVariance)
		fisherDefined := weightedVariance >= 0

		summary.TotalSupport = totalWeight
		summary.EffectivePeers = kish
		summary.SignedCorrelation = signedMean
		summary.AbsoluteCorrelation = absoluteMean
		summary.PeerEnergyRate = energyMean
		summary.Dispersion = dispersion
		summary.Defined = totalWeight > 0
		summary.FisherDefined = fisherDefined

		if !yield(unsafe.Pointer(&summary)) {
			return
		}
	}
}

/*
CohortSigned yields the support-weighted signed cohort correlation.
*/
type CohortSigned struct {
	*core.PrimitiveError

	out float64
}

func NewCohortSigned() *CohortSigned {
	return &CohortSigned{PrimitiveError: core.NewPrimitiveError()}
}

func (cohortSigned *CohortSigned) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)

			if !summary.Defined {
				continue
			}

			cohortSigned.out = summary.SignedCorrelation

			if !yield(unsafe.Pointer(&cohortSigned.out)) {
				return
			}
		}
	}
}

/*
CohortAbsolute yields the support-weighted absolute cohort correlation.
*/
type CohortAbsolute struct {
	*core.PrimitiveError

	out float64
}

func NewCohortAbsolute() *CohortAbsolute {
	return &CohortAbsolute{PrimitiveError: core.NewPrimitiveError()}
}

func (cohortAbsolute *CohortAbsolute) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)

			if !summary.Defined {
				continue
			}

			cohortAbsolute.out = summary.AbsoluteCorrelation

			if !yield(unsafe.Pointer(&cohortAbsolute.out)) {
				return
			}
		}
	}
}

/*
PeerCount yields the number of admitted cohort peers.
*/
type PeerCount struct {
	*core.PrimitiveError

	out float64
}

func NewPeerCount() *PeerCount {
	return &PeerCount{PrimitiveError: core.NewPrimitiveError()}
}

func (peerCount *PeerCount) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)
			peerCount.out = summary.Peers

			if !yield(unsafe.Pointer(&peerCount.out)) {
				return
			}
		}
	}
}

/*
EffectivePeers yields the Kish effective peer count.
*/
type EffectivePeers struct {
	*core.PrimitiveError

	out float64
}

func NewEffectivePeers() *EffectivePeers {
	return &EffectivePeers{PrimitiveError: core.NewPrimitiveError()}
}

func (effectivePeers *EffectivePeers) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)

			if !summary.Defined {
				continue
			}

			effectivePeers.out = summary.EffectivePeers

			if !yield(unsafe.Pointer(&effectivePeers.out)) {
				return
			}
		}
	}
}

/*
PeerEnergy yields the support-weighted peer energy rate.
*/
type PeerEnergy struct {
	*core.PrimitiveError

	out float64
}

func NewPeerEnergy() *PeerEnergy {
	return &PeerEnergy{PrimitiveError: core.NewPrimitiveError()}
}

func (peerEnergy *PeerEnergy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)

			if !summary.Defined {
				continue
			}

			peerEnergy.out = summary.PeerEnergyRate

			if !yield(unsafe.Pointer(&peerEnergy.out)) {
				return
			}
		}
	}
}

/*
Dispersion yields the Fisher-space cohort correlation dispersion.
*/
type Dispersion struct {
	*core.PrimitiveError

	out float64
}

func NewDispersion() *Dispersion {
	return &Dispersion{PrimitiveError: core.NewPrimitiveError()}
}

func (dispersion *Dispersion) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			summary := (*CohortSummary)(arriving)

			if !summary.FisherDefined {
				continue
			}

			dispersion.out = summary.Dispersion

			if !yield(unsafe.Pointer(&dispersion.out)) {
				return
			}
		}
	}
}
