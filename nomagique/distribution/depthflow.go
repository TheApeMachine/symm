package distribution

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type depthFlowSymbolState struct {
	prevBids              map[float64]float64
	prevAsks              map[float64]float64
	prevTotal             float64
	prevAtNano            float64
	hasPrev               bool
	hasPrevBookImb        bool
	prevBookImb           float64
	hasPrevGap            bool
	prevGap               float64
	bookImbMoments        core.Primitive
	bookImbResidual       core.Primitive
	gapMoments            core.Primitive
	gapResidual           core.Primitive
	turnoverMoments       core.Primitive
	turnoverResidual      core.Primitive
	netChangeMoments      core.Primitive
	netChangeResidual     core.Primitive
	signedFlowMoments     core.Primitive
	signedFlowResidual    core.Primitive
	historyPoints         [][2]float64
	historyDistances      []float64
}

func newDepthFlowSymbolState() *depthFlowSymbolState {
	return &depthFlowSymbolState{
		prevBids:           make(map[float64]float64),
		prevAsks:           make(map[float64]float64),
		bookImbMoments:     statistic.NewEstimator(),
		bookImbResidual:    statistic.NewCausalResidual(),
		gapMoments:         statistic.NewEstimator(),
		gapResidual:        statistic.NewCausalResidual(),
		turnoverMoments:    statistic.NewEstimator(),
		turnoverResidual:   statistic.NewCausalResidual(),
		netChangeMoments:   statistic.NewEstimator(),
		netChangeResidual:  statistic.NewCausalResidual(),
		signedFlowMoments:  statistic.NewEstimator(),
		signedFlowResidual: statistic.NewCausalResidual(),
	}
}

/*
DepthFlow measures order-book depth distribution asymmetry, touch resolution
gap, displayed depth additions and removals, and exposure-normalized flow rates.
Each arrival is *data.Message containing:
  Key: symbol string
  Value: sequence yielding:
    *[2][][2]float64{bids, asks}
    *float64 atNano
Yields 48 values: 47 output metrics matching depthflow outputKeys plus prevAtNano.
*/
type DepthFlow struct {
	*core.PrimitiveError
	symbols map[string]*depthFlowSymbolState
	out     [48]float64
}

func NewDepthFlow() core.Primitive {
	return &DepthFlow{
		PrimitiveError: core.NewPrimitiveError(),
		symbols:        make(map[string]*depthFlowSymbolState),
	}
}

func (op *DepthFlow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			var pairs [2][][2]float64
			var atNano float64
			var hasPairs, hasAt bool

			for ptr := range message.Value.Next(nil) {
				if !hasPairs {
					pairs = *(*[2][][2]float64)(ptr)
					hasPairs = true
					continue
				}
				if !hasAt {
					atNano = *(*float64)(ptr)
					hasAt = true
					continue
				}
			}

			if !hasPairs || !hasAt {
				op.Error(core.ErrShape)
				return
			}

			bids := pairs[0]
			asks := pairs[1]

			if len(bids) == 0 || len(asks) == 0 {
				return
			}

			touchBid := bids[0][0] * bids[0][1]
			touchAsk := asks[0][0] * asks[0][1]

			obsBid := 0.0
			for _, b := range bids {
				obsBid += b[0] * b[1]
			}

			obsAsk := 0.0
			for _, a := range asks {
				obsAsk += a[0] * a[1]
			}

			total := obsBid + obsAsk
			bookImb := (obsBid - obsAsk) / total
			touchImb := (touchBid - touchAsk) / (touchBid + touchAsk)
			gap := touchImb - bookImb
			gapDist := math.Abs(gap)

			state := op.symbols[symbol]
			if state == nil {
				state = newDepthFlowSymbolState()
				op.symbols[symbol] = state
			}

			currentBids := make(map[float64]float64, len(bids))
			for _, b := range bids {
				currentBids[b[0]] = b[0] * b[1]
			}

			currentAsks := make(map[float64]float64, len(asks))
			for _, a := range asks {
				currentAsks[a[0]] = a[0] * a[1]
			}

			addedBid, removedBid := 0.0, 0.0
			addedAsk, removedAsk := 0.0, 0.0
			timeDelta := 0.1
			outPrevAt := 0.0

			bookTurnoverRate := 0.0
			netBookChangeRate := 0.0
			signedNetFlowRate := 0.0

			addedRateBid := 0.0
			addedRateAsk := 0.0
			removedRateBid := 0.0
			removedRateAsk := 0.0
			netFlowRateBid := 0.0
			netFlowRateAsk := 0.0

			netBid := 0.0
			netAsk := 0.0
			flowActivityImbalance := 0.0

			if state.hasPrev {
				outPrevAt = state.prevAtNano
				if atNano > state.prevAtNano {
					timeDelta = (atNano - state.prevAtNano) * 1e-9
				}

				// Bid level mutations
				for price, notional := range currentBids {
					prev := state.prevBids[price]
					if notional > prev {
						addedBid += notional - prev
					} else if notional < prev {
						removedBid += prev - notional
					}
				}
				for price, prev := range state.prevBids {
					if _, ok := currentBids[price]; !ok {
						removedBid += prev
					}
				}

				// Ask level mutations
				for price, notional := range currentAsks {
					prev := state.prevAsks[price]
					if notional > prev {
						addedAsk += notional - prev
					} else if notional < prev {
						removedAsk += prev - notional
					}
				}
				for price, prev := range state.prevAsks {
					if _, ok := currentAsks[price]; !ok {
						removedAsk += prev
					}
				}

				netBid = addedBid - removedBid
				netAsk = addedAsk - removedAsk
				grossBid := addedBid + removedBid
				grossAsk := addedAsk + removedAsk
				bookActivity := grossBid + grossAsk
				netBookChange := total - state.prevTotal
				signedNetFlow := netBid - netAsk

				reference := (state.prevTotal + total) / 2.0
				if reference > 0 && timeDelta > 0 {
					bookTurnoverRate = bookActivity / (reference * timeDelta)
					netBookChangeRate = netBookChange / (reference * timeDelta)
					signedNetFlowRate = signedNetFlow / (reference * timeDelta)
				}

				if timeDelta > 0 {
					addedRateBid = addedBid / timeDelta
					addedRateAsk = addedAsk / timeDelta
					removedRateBid = removedBid / timeDelta
					removedRateAsk = removedAsk / timeDelta
					netFlowRateBid = netBid / timeDelta
					netFlowRateAsk = netAsk / timeDelta
				}

				denom := math.Abs(netBid) + math.Abs(netAsk)
				if denom > 0 {
					flowActivityImbalance = signedNetFlow / denom
				}
			}

			// Baselines: Book Imbalance
			var bReading [10]float64
			var bRes [8]float64
			for ptr := range state.bookImbMoments.Next(data.NewValue(bookImb).Next(nil)) {
				bReading = *(*[10]float64)(ptr)
				for rPtr := range state.bookImbResidual.Next(data.NewValue(bReading).Next(nil)) {
					bRes = *(*[8]float64)(rPtr)
				}
			}
			bookImbBaseline := 0.0
			bookImbDivergence := 0.0
			bookImbZScore := 0.0
			if bRes[0] == 1 {
				bookImbBaseline = bRes[1]
				bookImbDivergence = bRes[4]
				bookImbZScore = bRes[6]
			}
			bookImbVelocity := 0.0
			if state.hasPrevBookImb {
				bookImbVelocity = bookImb - state.prevBookImb
			}
			state.prevBookImb = bookImb
			state.hasPrevBookImb = true

			// Baselines: Resolution Gap
			var gReading [10]float64
			var gRes [8]float64
			for ptr := range state.gapMoments.Next(data.NewValue(gap).Next(nil)) {
				gReading = *(*[10]float64)(ptr)
				for rPtr := range state.gapResidual.Next(data.NewValue(gReading).Next(nil)) {
					gRes = *(*[8]float64)(rPtr)
				}
			}
			gapBaseline := 0.0
			gapDivergence := 0.0
			gapZScore := 0.0
			if gRes[0] == 1 {
				gapBaseline = gRes[1]
				gapDivergence = gRes[4]
				gapZScore = gRes[6]
			}
			gapVelocity := 0.0
			if state.hasPrevGap {
				gapVelocity = gap - state.prevGap
			}
			state.prevGap = gap
			state.hasPrevGap = true

			// Baselines: Turnover Rate
			turnoverBaseline := 0.0
			turnoverDivergence := 0.0
			turnoverZScore := 0.0
			turnoverRatio := 1.0
			if state.hasPrev {
				var tReading [10]float64
				var tRes [8]float64
				for ptr := range state.turnoverMoments.Next(data.NewValue(bookTurnoverRate).Next(nil)) {
					tReading = *(*[10]float64)(ptr)
					for rPtr := range state.turnoverResidual.Next(data.NewValue(tReading).Next(nil)) {
						tRes = *(*[8]float64)(rPtr)
					}
				}
				if tRes[0] == 1 {
					turnoverBaseline = tRes[1]
					turnoverDivergence = tRes[4]
					turnoverZScore = tRes[6]
					if turnoverBaseline > 0 {
						turnoverRatio = bookTurnoverRate / turnoverBaseline
					}
				}
			}

			// Baselines: Net Book Change Rate
			netChangeBaseline := 0.0
			netChangeDivergence := 0.0
			netChangeZScore := 0.0
			if state.hasPrev {
				var ncReading [10]float64
				var ncRes [8]float64
				for ptr := range state.netChangeMoments.Next(data.NewValue(netBookChangeRate).Next(nil)) {
					ncReading = *(*[10]float64)(ptr)
					for rPtr := range state.netChangeResidual.Next(data.NewValue(ncReading).Next(nil)) {
						ncRes = *(*[8]float64)(rPtr)
					}
				}
				if ncRes[0] == 1 {
					netChangeBaseline = ncRes[1]
					netChangeDivergence = ncRes[4]
					netChangeZScore = ncRes[6]
				}
			}

			// Baselines: Signed Net Flow Rate
			signedFlowBaseline := 0.0
			signedFlowDivergence := 0.0
			signedFlowZScore := 0.0
			if state.hasPrev {
				var sfReading [10]float64
				var sfRes [8]float64
				for ptr := range state.signedFlowMoments.Next(data.NewValue(signedNetFlowRate).Next(nil)) {
					sfReading = *(*[10]float64)(ptr)
					for rPtr := range state.signedFlowResidual.Next(data.NewValue(sfReading).Next(nil)) {
						sfRes = *(*[8]float64)(rPtr)
					}
				}
				if sfRes[0] == 1 {
					signedFlowBaseline = sfRes[1]
					signedFlowDivergence = sfRes[4]
					signedFlowZScore = sfRes[6]
				}
			}

			// History: nearest neighbor on [turnoverZScore, netChangeZScore]
			histDist := 0.0
			histPerc := 0.0
			if state.hasPrev {
				target := [2]float64{turnoverZScore, netChangeZScore}
				if len(state.historyPoints) == 0 {
					state.historyPoints = append(state.historyPoints, target)
				} else {
					diffX := target[0] - state.historyPoints[0][0]
					diffY := target[1] - state.historyPoints[0][1]
					minDist := math.Sqrt(diffX*diffX + diffY*diffY)

					for i := 1; i < len(state.historyPoints); i++ {
						dx := target[0] - state.historyPoints[i][0]
						dy := target[1] - state.historyPoints[i][1]
						d := math.Sqrt(dx*dx + dy*dy)
						if d < minDist {
							minDist = d
						}
					}

					if len(state.historyDistances) > 0 {
						below := 0
						for _, pd := range state.historyDistances {
							if pd <= minDist {
								below++
							}
						}
						histPerc = float64(below) / float64(len(state.historyDistances))
					}

					histDist = minDist
					state.historyDistances = append(state.historyDistances, minDist)
					state.historyPoints = append(state.historyPoints, target)
				}
			}

			// Advance state
			state.prevBids = currentBids
			state.prevAsks = currentAsks
			state.prevTotal = total
			state.prevAtNano = atNano
			state.hasPrev = true

			op.out[0] = obsBid
			op.out[1] = obsAsk
			op.out[2] = total
			op.out[3] = obsBid
			op.out[4] = obsAsk
			op.out[5] = total
			op.out[6] = bookImb
			op.out[7] = bookImb
			op.out[8] = touchImb
			op.out[9] = gap
			op.out[10] = gapDist
			op.out[11] = addedBid
			op.out[12] = removedBid
			op.out[13] = netBid
			op.out[14] = addedAsk
			op.out[15] = removedAsk
			op.out[16] = netAsk
			op.out[17] = flowActivityImbalance
			op.out[18] = bookImbBaseline
			op.out[19] = bookImbDivergence
			op.out[20] = bookImbZScore
			op.out[21] = gapBaseline
			op.out[22] = gapDivergence
			op.out[23] = gapZScore
			op.out[24] = bookImbVelocity
			op.out[25] = gapVelocity
			op.out[26] = addedRateBid
			op.out[27] = addedRateAsk
			op.out[28] = removedRateBid
			op.out[29] = removedRateAsk
			op.out[30] = netFlowRateBid
			op.out[31] = netFlowRateAsk
			op.out[32] = bookTurnoverRate
			op.out[33] = netBookChangeRate
			op.out[34] = signedNetFlowRate
			op.out[35] = turnoverBaseline
			op.out[36] = turnoverDivergence
			op.out[37] = turnoverZScore
			op.out[38] = turnoverRatio
			op.out[39] = netChangeBaseline
			op.out[40] = netChangeDivergence
			op.out[41] = netChangeZScore
			op.out[42] = signedFlowBaseline
			op.out[43] = signedFlowDivergence
			op.out[44] = signedFlowZScore
			op.out[45] = histDist
			op.out[46] = histPerc
			op.out[47] = outPrevAt

			for i := range op.out {
				if !yield(unsafe.Pointer(&op.out[i])) {
					return
				}
			}
		}
	}
}
