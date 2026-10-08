package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Pairs runs bivariate Hayashi-Yoshida cross-correlation against peer paths
in the PathStore, computing cohort aggregation, Fisher coordinates, relative
energy, causal baselines, and historical recurrence.
Yields 36 float64 values matching the correlation signal output contract.
*/
type Pairs struct {
	*core.PrimitiveError
	symbol          string
	store           *PathStore
	estimator       core.Primitive
	dependence      core.Primitive
	fisher          core.Primitive
	cohort          core.Primitive
	relative        core.Primitive
	fisherEstimator core.Primitive
	history         core.Primitive
	hasPrevCorr     bool
	prevCorr        float64
	hasPrevRel      bool
	prevRel         float64
	out             [36]float64
}

func NewPairs(symbol string, store *PathStore, estimator core.Primitive) core.Primitive {
	return &Pairs{
		PrimitiveError:  core.NewPrimitiveError(),
		symbol:          symbol,
		store:           store,
		estimator:       estimator,
		dependence:      NewDependence(estimator),
		fisher:          NewFisher(),
		cohort:          NewCohort(),
		relative:        NewRelative(),
		fisherEstimator: NewFisherEstimator(),
		history:         NewHistory(),
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var vals [2]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 2 {
				vals[idx] = *(*float64)(arriving)
				idx++
			}
		}

		price := vals[0]

		symbol := op.symbol
		if symbol == "" && op.store != nil {
			symbol = op.store.Current()
		}

		var focalPath [][2]float64
		if op.store != nil && symbol != "" {
			focalPath = op.store.paths[symbol]
		}

		clear(op.out[:])
		op.out[0] = price
		op.out[1] = float64(len(focalPath))

		peers := []string{}
		if op.store != nil && symbol != "" {
			peers = op.store.Peers(symbol)
		}

		if len(peers) == 0 || len(focalPath) < 2 {
			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
			return
		}

		// Evaluate dependence across all peers
		var peerInputs [][3]float64
		var lastDep [13]float64
		var validPeerCount float64

		for _, peer := range peers {
			peerPath := op.store.paths[peer]
			if len(peerPath) < 2 {
				continue
			}

			pair := [2][][2]float64{peerPath, focalPath}
			for ptr := range op.dependence.Next(data.NewValue(pair).Next(nil)) {
				lastDep = *(*[13]float64)(ptr)
			}

			corr := lastDep[0]
			supp := lastDep[2]
			peerEnergy := lastDep[8] // leftEnergyRate is reference (peer) rate

			peerInputs = append(peerInputs, [3]float64{corr, supp, peerEnergy})
			validPeerCount++
		}

		if validPeerCount == 0 {
			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
			return
		}

		// Cohort summary across peers
		var cohortRes [11]float64
		for ptr := range op.cohort.Next(data.NewValue(peerInputs...).Next(nil)) {
			cohortRes = *(*[11]float64)(ptr)
		}

		signedCorr := lastDep[0]
		absCorr := math.Abs(signedCorr)
		cov := lastDep[1]
		refEnergy := lastDep[3]
		measEnergy := lastDep[4]
		refEnergyRate := lastDep[8]
		measEnergyRate := lastDep[9]
		focalEnergyRate := measEnergyRate
		peerEnergyRate := refEnergyRate
		measReturns := lastDep[7]
		refReturns := lastDep[6]
		sharedTime := lastDep[11]
		overlapDensity := lastDep[12]
		overlapPairCount := lastDep[2]

		cohortPeerCount := cohortRes[1]
		cohortEffPeers := cohortRes[4]
		cohortSignedCorr := cohortRes[5]
		cohortAbsCorr := cohortRes[6]
		cohortDispersion := cohortRes[8]

		effSampleCount := overlapPairCount
		if cohortEffPeers > 0 {
			effSampleCount = cohortEffPeers
		}

		// Fisher p-value and standard error
		var fisherRes [6]float64
		for ptr := range op.fisher.Next(data.NewValue([3]float64{signedCorr, overlapPairCount, 1.0}).Next(nil)) {
			fisherRes = *(*[6]float64)(ptr)
		}
		corrPVal := fisherRes[1]
		corrSE := fisherRes[3]

		// Relative return energy
		relEnergy := 0.0
		if peerEnergyRate > 0 {
			relEnergy = focalEnergyRate / peerEnergyRate
		}
		relCohortEnergy := 0.0
		if cohortRes[7] > 0 {
			relCohortEnergy = focalEnergyRate / cohortRes[7]
		}

		// Correlation causal baseline
		var fisherEstRes [10]float64
		for ptr := range op.fisherEstimator.Next(data.NewValue(signedCorr).Next(nil)) {
			fisherEstRes = *(*[10]float64)(ptr)
		}
		corrBaseline := fisherEstRes[2]
		corrDivergence := fisherEstRes[3]
		corrZScore := fisherEstRes[6]

		corrVelocity := 0.0
		if op.hasPrevCorr {
			corrVelocity = signedCorr - op.prevCorr
		}
		op.prevCorr = signedCorr
		op.hasPrevCorr = true

		// Relative energy baseline
		var relRes [4]float64
		idxRel := 0
		for ptr := range op.relative.Next(data.NewValue(relEnergy).Next(nil)) {
			if idxRel < 4 {
				relRes[idxRel] = *(*float64)(ptr)
				idxRel++
			}
		}
		relBaseline := relRes[0]
		relDivergence := relRes[1]
		relZScore := relRes[2]

		relVelocity := 0.0
		if op.hasPrevRel {
			relVelocity = relEnergy - op.prevRel
		}
		op.prevRel = relEnergy
		op.hasPrevRel = true

		// Historical path distance and percentile
		var histRes [2]float64
		idxHist := 0
		for ptr := range op.history.Next(data.NewValue(corrZScore, relZScore).Next(nil)) {
			if idxHist < 2 {
				histRes[idxHist] = *(*float64)(ptr)
				idxHist++
			}
		}
		histDistance := histRes[0]
		histPercentile := histRes[1]

		op.out[0] = price
		op.out[1] = float64(len(focalPath))
		op.out[2] = signedCorr
		op.out[3] = absCorr
		op.out[4] = cohortSignedCorr
		op.out[5] = cohortAbsCorr
		op.out[6] = cov
		op.out[7] = refEnergy
		op.out[8] = measEnergy
		op.out[9] = refEnergyRate
		op.out[10] = measEnergyRate
		op.out[11] = peerEnergyRate
		op.out[12] = focalEnergyRate
		op.out[13] = measReturns
		op.out[14] = refReturns
		op.out[15] = sharedTime
		op.out[16] = overlapDensity
		op.out[17] = overlapPairCount
		op.out[18] = effSampleCount
		op.out[19] = corrPVal
		op.out[20] = corrSE
		op.out[21] = cohortPeerCount
		op.out[22] = cohortDispersion
		op.out[23] = cohortEffPeers
		op.out[24] = relEnergy
		op.out[25] = relCohortEnergy
		op.out[26] = corrBaseline
		op.out[27] = corrDivergence
		op.out[28] = corrZScore
		op.out[29] = corrVelocity
		op.out[30] = relBaseline
		op.out[31] = relDivergence
		op.out[32] = relZScore
		op.out[33] = relVelocity
		op.out[34] = histDistance
		op.out[35] = histPercentile

		for i := range op.out {
			if !yield(unsafe.Pointer(&op.out[i])) {
				return
			}
		}
	}
}
