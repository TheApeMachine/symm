package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
minimumCohortSupport is the smallest overlap that can carry a correlation at
all: one overlapping pair states nothing about co-movement.
*/
const minimumCohortSupport = 2

/*
Cohort accumulates many pairwise correlations into one cross-sectional view
of how a focal path relates to its peers.

The carrier in is one peer's correlation; the carrier out is the running
support-weighted signed correlation. Each peer is weighted by its own overlap
support, read from the Support slot, so a peer that genuinely co-sampled
counts for more than one that barely overlapped.

Dispersion accumulates in Fisher space, where the sampling variance of a
correlation is stationary; averaging raw correlations near ±1 understates
the spread.

Degenerate behavior: an omitted Support slot cannot weight a peer and folds
nothing. Reset clears the accumulator between cross-sections.
*/
type Cohort struct {
	Support    types.Node
	PeerEnergy types.Node

	totalSupport          types.Number
	weightedSigned        types.Number
	weightedAbsolute      types.Number
	weightedPeerEnergy    types.Number
	weightedFisher        types.Number
	weightedFisherSquared types.Number
	weightSquares         types.Number
	peers                 int
}

func (cohort *Cohort) Step(correlation types.Number) types.Number {
	if cohort.Support == nil {
		return cohort.SignedCorrelation()
	}

	support := cohort.Support.Step(correlation)
	value := float64(correlation)

	if support < minimumCohortSupport ||
		math.IsNaN(value) || math.IsInf(value, 0) {
		return cohort.SignedCorrelation()
	}

	var peerEnergy types.Number

	if cohort.PeerEnergy != nil {
		peerEnergy = cohort.PeerEnergy.Step(correlation)
	}

	fisher := math.Atanh(saturate(value))

	cohort.totalSupport += support
	cohort.weightedSigned += correlation * support
	cohort.weightedAbsolute += types.Number(math.Abs(value)) * support
	cohort.weightedPeerEnergy += peerEnergy * support
	cohort.weightedFisher += types.Number(fisher) * support
	cohort.weightedFisherSquared += types.Number(fisher*fisher) * support
	cohort.weightSquares += support * support
	cohort.peers++

	return cohort.SignedCorrelation()
}

// Reset clears the accumulator so one instance serves successive cross-sections.
func (cohort *Cohort) Reset() {
	cohort.totalSupport = 0
	cohort.weightedSigned = 0
	cohort.weightedAbsolute = 0
	cohort.weightedPeerEnergy = 0
	cohort.weightedFisher = 0
	cohort.weightedFisherSquared = 0
	cohort.weightSquares = 0
	cohort.peers = 0
}

// Ready reports whether any peer contributed enough support.
func (cohort *Cohort) Ready() bool {
	return cohort.peers > 0 && cohort.totalSupport > 0
}

// Peers returns how many peers contributed.
func (cohort *Cohort) Peers() types.Number { return types.Number(cohort.peers) }

// TotalSupport returns the summed overlap support across every peer.
func (cohort *Cohort) TotalSupport() types.Number { return cohort.totalSupport }

/*
SignedCorrelation returns the support-weighted mean correlation: whether the
focal path moves with or against its cohort on balance.
*/
func (cohort *Cohort) SignedCorrelation() types.Number {
	if cohort.totalSupport <= 0 {
		return 0
	}

	return cohort.weightedSigned / cohort.totalSupport
}

/*
AbsoluteCorrelation returns the support-weighted mean magnitude: how strongly
the focal path is coupled to its cohort regardless of direction.
*/
func (cohort *Cohort) AbsoluteCorrelation() types.Number {
	if cohort.totalSupport <= 0 {
		return 0
	}

	return cohort.weightedAbsolute / cohort.totalSupport
}

// PeerEnergyRate returns the support-weighted mean return energy of the peers.
func (cohort *Cohort) PeerEnergyRate() types.Number {
	if cohort.totalSupport <= 0 {
		return 0
	}

	return cohort.weightedPeerEnergy / cohort.totalSupport
}

/*
Dispersion returns the support-weighted standard deviation of the peer
correlations in Fisher space: whether the cohort agrees about the focal path
or is split.
*/
func (cohort *Cohort) Dispersion() types.Number {
	if cohort.totalSupport <= 0 {
		return 0
	}

	mean := cohort.weightedFisher / cohort.totalSupport
	variance := cohort.weightedFisherSquared/cohort.totalSupport - mean*mean

	if variance <= 0 {
		return 0
	}

	return types.Number(math.Sqrt(float64(variance)))
}

/*
EffectivePeers returns Kish's effective sample size over the peer weights:
how many equally-weighted peers the support-weighted cohort is worth. A
cohort dominated by one heavily-overlapping peer reports close to one however
many peers it nominally holds.
*/
func (cohort *Cohort) EffectivePeers() types.Number {
	if cohort.weightSquares <= 0 {
		return 0
	}

	return cohort.totalSupport * cohort.totalSupport / cohort.weightSquares
}

/*
saturate nudges a correlation of exactly ±1 just inside the open interval so
the Fisher transform stays finite. A perfect correlation on a finite
asynchronous sample is a sampling artefact, not infinite evidence.
*/
func saturate(value float64) float64 {
	if value >= 1 {
		return math.Nextafter(1, 0)
	}

	if value <= -1 {
		return math.Nextafter(-1, 0)
	}

	return value
}

var _ types.Node = (*Cohort)(nil)
