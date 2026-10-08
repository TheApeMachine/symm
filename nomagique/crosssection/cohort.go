package crosssection

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type memberState struct {
	lastPrice float64
	lastAt    float64
	prevPrice float64
	prevAt    float64
	spacings  []float64
	hasPrev   bool
}

/*
Cohort measures cross-sectional price state across an explicitly supplied
market cohort over an adaptive event-time horizon.
*/
type Cohort struct {
	*core.PrimitiveError
	members            map[string]*memberState
	breadthMoments     core.Primitive
	breadthResidual    core.Primitive
	medianMoments      core.Primitive
	medianResidual     core.Primitive
	historyPoints      [][2]float64
	historyDistances   []float64
	hasPrevBreadth     bool
	prevBreadth        float64
	hasPrevMedian      bool
	prevMedian         float64
	out                [50]float64
}

func NewCohort() core.Primitive {
	return &Cohort{
		PrimitiveError:  core.NewPrimitiveError(),
		members:         make(map[string]*memberState),
		breadthMoments:  statistic.NewEstimator(),
		breadthResidual: statistic.NewCausalResidual(),
		medianMoments:   statistic.NewEstimator(),
		medianResidual:  statistic.NewCausalResidual(),
	}
}

func (op *Cohort) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			message := (*data.Message)(arriving)
			symbol := message.Key
			if symbol == "" {
				op.Error(core.ErrDomain)
				return
			}

			var vals [2]float64
			idx := 0
			for ptr := range message.Value.Next(nil) {
				if idx < 2 {
					vals[idx] = *(*float64)(ptr)
					idx++
				}
			}

			if idx < 2 {
				op.Error(core.ErrShape)
				return
			}

			price := vals[0]
			atNano := vals[1]

			m := op.members[symbol]
			if m == nil {
				m = &memberState{}
				op.members[symbol] = m
			}

			if m.hasPrev && atNano > m.lastAt {
				spacing := (atNano - m.lastAt) * 1e-9
				m.spacings = append(m.spacings, spacing)
				m.prevPrice = m.lastPrice
				m.prevAt = m.lastAt
			}
			m.lastPrice = price
			m.lastAt = atNano
			m.hasPrev = true

			// Estimate common horizon H as median of member cadences
			var cadences []float64
			for _, mem := range op.members {
				if len(mem.spacings) > 0 {
					sorted := slices.Clone(mem.spacings)
					slices.Sort(sorted)
					med := sorted[len(sorted)/2]
					cadences = append(cadences, med)
				}
			}

			horizon := 0.1 // default 100ms
			if len(cadences) > 0 {
				slices.Sort(cadences)
				horizon = cadences[len(cadences)/2]
			}

			// Gather valid member returns over step
			type memberReturn struct {
				symbol string
				ret    float64
				asofAge float64
				fromAge float64
			}
			var valid []memberReturn

			for s, mem := range op.members {
				if mem.prevPrice > 0 && mem.lastPrice > 0 && mem.prevAt < mem.lastAt {
					ret := math.Log(mem.lastPrice / mem.prevPrice)
					asofAge := (atNano - mem.lastAt) * 1e-9
					fromAge := (atNano - mem.prevAt) * 1e-9
					valid = append(valid, memberReturn{
						symbol: s,
						ret: ret,
						asofAge: asofAge,
						fromAge: fromAge,
					})
				}
			}

			cohortMemberCount := float64(len(op.members))
			validCount := float64(len(valid))
			excludedCount := cohortMemberCount - validCount

			clear(op.out[:])
			op.out[0] = cohortMemberCount
			op.out[1] = validCount
			op.out[2] = excludedCount
			op.out[3] = horizon

			if validCount == 0 {
				for i := range op.out {
					if !yield(unsafe.Pointer(&op.out[i])) {
						return
					}
				}
				continue
			}

			// Find focal member return
			focalRet := 0.0
			focalAsofAge := 0.0
			focalFromAge := 0.0
			for _, vr := range valid {
				if vr.symbol == symbol {
					focalRet = vr.ret
					focalAsofAge = vr.asofAge
					focalFromAge = vr.fromAge
					break
				}
			}

			// Advance, decline, unchanged
			advanceCount := 0.0
			declineCount := 0.0
			unchangedCount := 0.0
			var returns []float64
			var absReturns []float64

			for _, vr := range valid {
				returns = append(returns, vr.ret)
				absReturns = append(absReturns, math.Abs(vr.ret))

				if vr.ret > 0 {
					advanceCount++
				} else if vr.ret < 0 {
					declineCount++
				} else {
					unchangedCount++
				}
			}

			advFrac := advanceCount / validCount
			decFrac := declineCount / validCount
			unchFrac := unchangedCount / validCount
			dirPart := (advanceCount + declineCount) / validCount
			breadth := (advanceCount - declineCount) / validCount

			dirMoving := advanceCount + declineCount
			dirAgree := 0.0
			dirConsensus := 0.0
			if dirMoving > 0 {
				dirAgree = math.Max(advanceCount, declineCount) / dirMoving
				dirConsensus = math.Abs(advanceCount-declineCount) / dirMoving
			}

			// Medians and dispersions
			slices.Sort(returns)
			slices.Sort(absReturns)

			medRet := returns[len(returns)/2]
			medAbsRet := absReturns[len(absReturns)/2]

			sumAbs := 0.0
			sumSq := 0.0
			for _, r := range returns {
				sumAbs += math.Abs(r)
				sumSq += r * r
			}
			meanAbsRet := sumAbs / validCount
			rmsRet := math.Sqrt(sumSq / validCount)

			var retDevs []float64
			var magDevs []float64
			for _, r := range returns {
				retDevs = append(retDevs, math.Abs(r-medRet))
				magDevs = append(magDevs, math.Abs(math.Abs(r)-medAbsRet))
			}
			slices.Sort(retDevs)
			slices.Sort(magDevs)
			retMAD := retDevs[len(retDevs)/2]
			magMAD := magDevs[len(magDevs)/2]

			q25 := returns[int(float64(len(returns))*0.25)]
			q75 := returns[int(float64(len(returns))*0.75)]
			iqr := q75 - q25

			// Largest move
			largestAbs := absReturns[len(absReturns)-1]
			tieCount := 0.0
			largestSigned := 0.0
			largestSymbol := ""

			for _, vr := range valid {
				if math.Abs(vr.ret) == largestAbs {
					tieCount++
					largestSigned = vr.ret
					largestSymbol = vr.symbol
				}
			}

			largestShare := 0.0
			if sumAbs > 0 {
				largestShare = largestAbs / sumAbs
			}

			// Peer metrics relative to largest mover
			var peerAbsReturns []float64
			sameDirPeer := 0.0
			oppDirPeer := 0.0
			zeroDirPeer := 0.0

			for _, vr := range valid {
				if vr.symbol == largestSymbol {
					continue
				}
				peerAbsReturns = append(peerAbsReturns, math.Abs(vr.ret))

				if vr.ret == 0 {
					zeroDirPeer++
				} else if largestSigned > 0 && vr.ret > 0 {
					sameDirPeer++
				} else if largestSigned < 0 && vr.ret < 0 {
					sameDirPeer++
				} else {
					oppDirPeer++
				}
			}

			peerCount := float64(len(peerAbsReturns))
			peerMedAbs := 0.0
			peerMagMAD := 0.0
			largestExcess := 0.0
			largestRatio := 1.0
			largestMADExcess := 0.0
			sameDirFrac := 0.0
			oppDirFrac := 0.0
			zeroDirFrac := 0.0

			if peerCount > 0 {
				slices.Sort(peerAbsReturns)
				peerMedAbs = peerAbsReturns[len(peerAbsReturns)/2]

				var peerDevs []float64
				for _, pr := range peerAbsReturns {
					peerDevs = append(peerDevs, math.Abs(pr-peerMedAbs))
				}
				slices.Sort(peerDevs)
				peerMagMAD = peerDevs[len(peerDevs)/2]

				largestExcess = largestAbs - peerMedAbs
				if peerMedAbs > 0 {
					largestRatio = largestAbs / peerMedAbs
				}
				if peerMagMAD > 0 {
					largestMADExcess = largestExcess / peerMagMAD
				}

				sameDirFrac = sameDirPeer / peerCount
				oppDirFrac = oppDirPeer / peerCount
				zeroDirFrac = zeroDirPeer / peerCount
			}

			// Baselines: Breadth
			var bReading [10]float64
			var bRes [8]float64
			for ptr := range op.breadthMoments.Next(data.NewValue(breadth).Next(nil)) {
				bReading = *(*[10]float64)(ptr)
				for rPtr := range op.breadthResidual.Next(data.NewValue(bReading).Next(nil)) {
					bRes = *(*[8]float64)(rPtr)
				}
			}
			bBaseline := 0.0
			bDiv := 0.0
			bZScore := 0.0
			if bRes[0] == 1 {
				bBaseline = bRes[1]
				bDiv = breadth - bBaseline
				bZScore = bRes[6]
			}
			bVelocity := 0.0
			if op.hasPrevBreadth {
				bVelocity = breadth - op.prevBreadth
			}
			op.prevBreadth = breadth
			op.hasPrevBreadth = true

			// Baselines: Median Return
			var mReading [10]float64
			var mRes [8]float64
			for ptr := range op.medianMoments.Next(data.NewValue(medRet).Next(nil)) {
				mReading = *(*[10]float64)(ptr)
				for rPtr := range op.medianResidual.Next(data.NewValue(mReading).Next(nil)) {
					mRes = *(*[8]float64)(rPtr)
				}
			}
			mBaseline := 0.0
			mDiv := 0.0
			mZScore := 0.0
			if mRes[0] == 1 {
				mBaseline = mRes[1]
				mDiv = medRet - mBaseline
				mZScore = mRes[6]
			}
			mVelocity := 0.0
			if op.hasPrevMedian {
				mVelocity = medRet - op.prevMedian
			}
			op.prevMedian = medRet
			op.hasPrevMedian = true

			// Historical recurrence
			target := [2]float64{bZScore, mZScore}
			histDist := 0.0
			histPerc := 0.0

			if len(op.historyPoints) == 0 {
				op.historyPoints = append(op.historyPoints, target)
			} else {
				diffX := target[0] - op.historyPoints[0][0]
				diffY := target[1] - op.historyPoints[0][1]
				minDist := math.Sqrt(diffX*diffX + diffY*diffY)

				for i := 1; i < len(op.historyPoints); i++ {
					dx := target[0] - op.historyPoints[i][0]
					dy := target[1] - op.historyPoints[i][1]
					d := math.Sqrt(dx*dx + dy*dy)
					if d < minDist {
						minDist = d
					}
				}

				if len(op.historyDistances) > 0 {
					below := 0
					for _, pd := range op.historyDistances {
						if pd <= minDist {
							below++
						}
					}
					histPerc = float64(below) / float64(len(op.historyDistances))
				}

				histDist = minDist
				op.historyDistances = append(op.historyDistances, minDist)
				op.historyPoints = append(op.historyPoints, target)
			}

			op.out[0] = cohortMemberCount
			op.out[1] = validCount
			op.out[2] = excludedCount
			op.out[3] = horizon
			op.out[4] = focalRet
			op.out[5] = math.Abs(focalRet)
			op.out[6] = focalAsofAge
			op.out[7] = focalFromAge
			op.out[8] = advanceCount
			op.out[9] = declineCount
			op.out[10] = unchangedCount
			op.out[11] = advFrac
			op.out[12] = decFrac
			op.out[13] = unchFrac
			op.out[14] = dirPart
			op.out[15] = breadth
			op.out[16] = dirAgree
			op.out[17] = dirConsensus
			op.out[18] = medRet
			op.out[19] = medAbsRet
			op.out[20] = meanAbsRet
			op.out[21] = rmsRet
			op.out[22] = retMAD
			op.out[23] = magMAD
			op.out[24] = iqr
			op.out[25] = tieCount
			op.out[26] = largestAbs
			op.out[27] = largestSigned
			op.out[28] = largestShare
			op.out[29] = peerMedAbs
			op.out[30] = peerMagMAD
			op.out[31] = largestExcess
			op.out[32] = largestRatio
			op.out[33] = largestMADExcess
			op.out[34] = sameDirPeer
			op.out[35] = oppDirPeer
			op.out[36] = zeroDirPeer
			op.out[37] = sameDirFrac
			op.out[38] = oppDirFrac
			op.out[39] = zeroDirFrac
			op.out[40] = bBaseline
			op.out[41] = bDiv
			op.out[42] = bZScore
			op.out[43] = mBaseline
			op.out[44] = mDiv
			op.out[45] = mZScore
			op.out[46] = mVelocity
			op.out[47] = bVelocity
			op.out[48] = histDist
			op.out[49] = histPerc

			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
		}
	}
}
