package temporal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Activity composes VolumeBar tape accounting with book touch metrics,
causal residual baselines, and historical trajectory recurrence.
Operands arrive in order: [price, qty, atNanos, bid, ask].
Yields 41 values: 40 metrics matching outputKeys plus outBarStartNanos.
*/
type Activity struct {
	*core.PrimitiveError
	volumeBar         core.Primitive
	notionalMoments   core.Primitive
	notionalResidual  core.Primitive
	spreadMoments     core.Primitive
	spreadResidual    core.Primitive
	midReturnMoments  core.Primitive
	midReturnResidual core.Primitive
	historyPoints     [][2]float64
	historyDistances  []float64
	hasPrevNotional   bool
	prevNotionalRate  float64
	hasPrevSpreadDiv  bool
	prevSpreadDiv     float64
	hasPrevMidReturn  bool
	prevMidReturn     float64
	out               [41]float64
}

func NewActivity(target ...float64) core.Primitive {
	return &Activity{
		PrimitiveError:    core.NewPrimitiveError(),
		volumeBar:         NewVolumeBar(target...),
		notionalMoments:   statistic.NewEstimator(),
		notionalResidual:  statistic.NewCausalResidual(),
		spreadMoments:     statistic.NewEstimator(),
		spreadResidual:    statistic.NewCausalResidual(),
		midReturnMoments:  statistic.NewEstimator(),
		midReturnResidual: statistic.NewCausalResidual(),
	}
}

func (op *Activity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var inputs [5]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 5 {
				inputs[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 5 {
			op.Error(core.ErrShape)
			return
		}

		price := inputs[0]
		qty := inputs[1]
		atNanos := inputs[2]
		bid := inputs[3]
		ask := inputs[4]

		hasTouch := bid > 0 && ask > 0
		midpoint := 0.0
		spread := 0.0
		relSpread := 0.0

		if hasTouch {
			midpoint = (bid + ask) * 0.5
			spread = ask - bid
			if midpoint > 0 {
				relSpread = spread / midpoint
			}
		}

		// Run VolumeBar with midpoint (or price if no book, so returns stay zero)
		vbMidpoint := midpoint
		if !hasTouch {
			vbMidpoint = 0.0
		}

		var vb [18]float64
		vbIdx := 0
		for ptr := range op.volumeBar.Next(data.NewValue(price, qty, atNanos, vbMidpoint).Next(nil)) {
			if vbIdx < 18 {
				vb[vbIdx] = *(*float64)(ptr)
				vbIdx++
			}
		}

		if vbIdx < 18 {
			op.Error(core.ErrShape)
			return
		}

		tradeNotional := vb[0]
		tradeInterval := vb[1]
		barTarget := vb[2]
		barQty := vb[3]
		barNotional := vb[4]
		barTradeCount := vb[5]
		barDuration := vb[6]
		volumeRate := vb[7]
		notionalRate := vb[8]
		tradeRate := vb[9]
		completedBars := vb[10]
		fromMid := vb[11]
		atMid := vb[12]
		midReturn := vb[13]
		midReturnRate := vb[14]
		posReturn := vb[15]
		negReturn := vb[16]
		outBarStart := vb[17]

		// Notional rate baseline & divergence
		notionalBaseline := 0.0
		notionalRatio := 1.0
		notionalDivergence := 0.0
		notionalZScore := 0.0

		var notionalReading statistic.MomentReading
		var notionalRes statistic.CausalResidualResult
		for ptr := range op.notionalMoments.Next(data.NewValue(notionalRate).Next(nil)) {
			notionalReading = *(*statistic.MomentReading)(ptr)
			for rPtr := range op.notionalResidual.Next(data.NewValue(notionalReading).Next(nil)) {
				notionalRes = *(*statistic.CausalResidualResult)(rPtr)
			}
		}

		if notionalRes.HasPrior {
			notionalBaseline = notionalRes.Baseline
			if notionalBaseline > 0 && notionalRate > 0 {
				notionalRatio = notionalRate / notionalBaseline
				notionalDivergence = math.Log(notionalRatio)
			}
			notionalZScore = notionalRes.ZScore
		}

		notionalVelocity := 0.0
		if op.hasPrevNotional {
			notionalVelocity = notionalRate - op.prevNotionalRate
		}
		op.prevNotionalRate = notionalRate
		op.hasPrevNotional = true

		// Spread metrics & baselines
		spreadBaseline := 0.0
		spreadRatio := 0.0
		spreadDivergence := 0.0
		spreadZScore := 0.0
		relSpreadBaseline := 0.0

		if hasTouch && spread > 0 {
			spreadRatio = 1.0
			var spreadReading statistic.MomentReading
			var spreadRes statistic.CausalResidualResult
			for ptr := range op.spreadMoments.Next(data.NewValue(spread).Next(nil)) {
				spreadReading = *(*statistic.MomentReading)(ptr)
				for rPtr := range op.spreadResidual.Next(data.NewValue(spreadReading).Next(nil)) {
					spreadRes = *(*statistic.CausalResidualResult)(rPtr)
				}
			}

			if spreadRes.HasPrior {
				spreadBaseline = spreadRes.Baseline
				if spreadBaseline > 0 {
					spreadRatio = spread / spreadBaseline
					spreadDivergence = math.Log(spreadRatio)
				}
				disp := 0.0
				if spreadRes.PriorVariance > 0 {
					disp = math.Sqrt(spreadRes.PriorVariance)
				}
				if disp > 0 {
					spreadZScore = (spread - spreadBaseline) / disp
				} else {
					spreadZScore = spreadDivergence
				}
				if midpoint > 0 {
					relSpreadBaseline = spreadBaseline / midpoint
				}
			}
		}

		spreadDivVelocity := 0.0
		if op.hasPrevSpreadDiv {
			spreadDivVelocity = spreadDivergence - op.prevSpreadDiv
		}
		op.prevSpreadDiv = spreadDivergence
		op.hasPrevSpreadDiv = true

		// Midpoint return baseline
		midReturnBaseline := 0.0
		midReturnDivergence := 0.0
		midReturnZScore := 0.0

		if hasTouch {
			var midReading statistic.MomentReading
			var midRes statistic.CausalResidualResult
			for ptr := range op.midReturnMoments.Next(data.NewValue(midReturn).Next(nil)) {
				midReading = *(*statistic.MomentReading)(ptr)
				for rPtr := range op.midReturnResidual.Next(data.NewValue(midReading).Next(nil)) {
					midRes = *(*statistic.CausalResidualResult)(rPtr)
				}
			}

			if midRes.HasPrior {
				midReturnBaseline = midRes.Baseline
				midReturnDivergence = midRes.Residual
				midReturnZScore = midRes.ZScore
			}
		}

		midReturnVelocity := 0.0
		if op.hasPrevMidReturn {
			midReturnVelocity = midReturn - op.prevMidReturn
		}
		op.prevMidReturn = midReturn
		op.hasPrevMidReturn = true

		// Historical path distance and percentile on [spreadZScore, notionalZScore]
		target := [2]float64{spreadZScore, notionalZScore}
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

		op.out[0] = price
		op.out[1] = qty
		op.out[2] = tradeNotional
		op.out[3] = tradeInterval
		op.out[4] = barTarget
		op.out[5] = barQty
		op.out[6] = barNotional
		op.out[7] = barTradeCount
		op.out[8] = barDuration
		op.out[9] = volumeRate
		op.out[10] = notionalRate
		op.out[11] = tradeRate
		op.out[12] = completedBars
		op.out[13] = notionalBaseline
		op.out[14] = notionalRatio
		op.out[15] = notionalDivergence
		op.out[16] = notionalZScore
		op.out[17] = notionalVelocity
		op.out[18] = bid
		op.out[19] = ask
		op.out[20] = midpoint
		op.out[21] = spread
		op.out[22] = relSpread
		op.out[23] = relSpreadBaseline
		op.out[24] = spreadRatio
		op.out[25] = spreadDivergence
		op.out[26] = spreadZScore
		op.out[27] = spreadDivVelocity
		op.out[28] = fromMid
		op.out[29] = atMid
		op.out[30] = midReturn
		op.out[31] = midReturnRate
		op.out[32] = posReturn
		op.out[33] = negReturn
		op.out[34] = midReturnBaseline
		op.out[35] = midReturnDivergence
		op.out[36] = midReturnZScore
		op.out[37] = midReturnVelocity
		op.out[38] = histDist
		op.out[39] = histPerc
		op.out[40] = outBarStart

		for i := range op.out {
			if !yield(unsafe.Pointer(&op.out[i])) {
				return
			}
		}
	}
}
