package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type peerState struct {
	lagMoments       core.Primitive
	lagResidual      core.Primitive
	corrEst          core.Primitive
	gainMoments      core.Primitive
	gainResidual     core.Primitive
	history          core.Primitive
	hasPrevLag       bool
	prevLag          float64
	hasPrevGain      bool
	prevGain         float64
}

func newPeerState() *peerState {
	return &peerState{
		lagMoments:   statistic.NewEstimator(),
		lagResidual:  statistic.NewCausalResidual(),
		corrEst:      NewFisherEstimator(),
		gainMoments:  statistic.NewEstimator(),
		gainResidual: statistic.NewCausalResidual(),
		history:      NewHistory(),
	}
}

/*
Leads runs exact discrete lag search of the current symbol against every peer
path in PathStore.
For each peer, yields 30 float64 values matching leadLagPeerFactKeys.
*/
type Leads struct {
	*core.PrimitiveError
	symbol    string
	store     *PathStore
	estimator core.Primitive
	leadlag   core.Primitive
	fisher    core.Primitive
	peers     map[string]*peerState
	out       [30]float64
}

func NewLeads(symbol string, store *PathStore, estimator core.Primitive) core.Primitive {
	return &Leads{
		PrimitiveError: core.NewPrimitiveError(),
		symbol:         symbol,
		store:          store,
		estimator:      estimator,
		leadlag:        NewLeadLag(estimator),
		fisher:         NewFisher(),
		peers:          make(map[string]*peerState),
	}
}

func (op *Leads) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

		symbol := op.symbol
		if symbol == "" && op.store != nil {
			symbol = op.store.Current()
		}

		if symbol == "" || op.store == nil {
			return
		}

		focalPath := op.store.paths[symbol]
		if len(focalPath) < 3 {
			return
		}

		peers := op.store.Peers(symbol)
		for _, peer := range peers {
			refPath := op.store.paths[peer]
			if len(refPath) < 3 {
				continue
			}

			state := op.peers[peer]
			if state == nil {
				state = newPeerState()
				op.peers[peer] = state
			}

			pair := [2][][2]float64{refPath, focalPath}
			var leadOut [2][]float64
			for ptr := range op.leadlag.Next(data.NewValue(pair).Next(nil)) {
				leadOut = *(*[2][]float64)(ptr)
			}

			summary := leadOut[0]
			if len(summary) < 22 || summary[5] == 0 {
				continue
			}

			bestLagCorr := summary[9]
			bestLagSec := summary[8]
			bestLagIdx := summary[7]
			contempCorr := summary[12]
			searchCount := summary[13]
			spacing := summary[14]
			span := summary[15]
			absGain := summary[18]
			lagFraction := summary[19]
			prominence := summary[20]
			curvature := summary[21]
			overlapPairs := summary[2]

			// Fisher inference
			var fisherRes [6]float64
			for ptr := range op.fisher.Next(data.NewValue([3]float64{bestLagCorr, overlapPairs, searchCount}).Next(nil)) {
				fisherRes = *(*[6]float64)(ptr)
			}
			corrPVal := fisherRes[1]
			searchAdjPVal := fisherRes[4]

			// Lag seconds baseline and residual
			var lagReading [10]float64
			var lagRes [8]float64
			for ptr := range state.lagMoments.Next(data.NewValue(bestLagSec).Next(nil)) {
				lagReading = *(*[10]float64)(ptr)
				for rPtr := range state.lagResidual.Next(data.NewValue(lagReading).Next(nil)) {
					lagRes = *(*[8]float64)(rPtr)
				}
			}
			lagBaseline := 0.0
			lagDivergence := 0.0
			lagNoiseScale := 0.0
			lagZScore := 0.0
			if lagRes[0] == 1 {
				lagBaseline = lagRes[1]
				lagNoiseScale = math.Sqrt(lagRes[2])
				lagDivergence = lagRes[4]
				lagZScore = lagRes[6]
			}

			// Best lag correlation baseline
			var corrEstRes [10]float64
			for ptr := range state.corrEst.Next(data.NewValue(bestLagCorr).Next(nil)) {
				corrEstRes = *(*[10]float64)(ptr)
			}
			corrBaseline := corrEstRes[2]
			corrZScore := corrEstRes[6]

			// Correlation gain baseline
			var gainReading [10]float64
			var gainRes [8]float64
			for ptr := range state.gainMoments.Next(data.NewValue(absGain).Next(nil)) {
				gainReading = *(*[10]float64)(ptr)
				for rPtr := range state.gainResidual.Next(data.NewValue(gainReading).Next(nil)) {
					gainRes = *(*[8]float64)(rPtr)
				}
			}
			gainBaseline := 0.0
			gainZScore := 0.0
			if gainRes[0] == 1 {
				gainBaseline = gainRes[1]
				gainZScore = gainRes[6]
			}

			// Velocities
			lagVelocity := 0.0
			if state.hasPrevLag {
				lagVelocity = bestLagSec - state.prevLag
			}
			state.prevLag = bestLagSec
			state.hasPrevLag = true

			gainVelocity := 0.0
			if state.hasPrevGain {
				gainVelocity = absGain - state.prevGain
			}
			state.prevGain = absGain
			state.hasPrevGain = true

			// History
			var histRes [2]float64
			hIdx := 0
			for ptr := range state.history.Next(data.NewValue(lagZScore, gainZScore).Next(nil)) {
				if hIdx < 2 {
					histRes[hIdx] = *(*float64)(ptr)
					hIdx++
				}
			}
			histDist := histRes[0]
			histPerc := histRes[1]

			op.out[0] = 1.0 // reference_symbol flag
			op.out[1] = contempCorr
			op.out[2] = bestLagCorr
			op.out[3] = bestLagSec
			op.out[4] = bestLagIdx
			op.out[5] = absGain
			op.out[6] = spacing * 1e-9
			op.out[7] = span
			op.out[8] = lagFraction
			op.out[9] = float64(len(refPath) - 1)
			op.out[10] = float64(len(focalPath) - 1)
			op.out[11] = overlapPairs
			op.out[12] = searchCount
			op.out[13] = overlapPairs
			op.out[14] = prominence
			op.out[15] = curvature
			op.out[16] = corrPVal
			op.out[17] = searchAdjPVal
			op.out[18] = lagBaseline
			op.out[19] = lagDivergence
			op.out[20] = lagNoiseScale
			op.out[21] = lagZScore
			op.out[22] = corrBaseline
			op.out[23] = corrZScore
			op.out[24] = gainBaseline
			op.out[25] = gainZScore
			op.out[26] = lagVelocity
			op.out[27] = gainVelocity
			op.out[28] = histDist
			op.out[29] = histPerc

			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
		}
	}
}
