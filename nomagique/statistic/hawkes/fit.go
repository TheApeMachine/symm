package hawkes

import (
	"iter"
	"math"
	"sort"
	"unsafe"

	"gonum.org/v1/gonum/optimize"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fit implements continuous-time maximum likelihood estimation of bivariate Hawkes
processes as a Primitive. It uses multi-start L-BFGS optimization with analytical
gradients, softplus coordinate transformations, and data-derived bounds from
empirical inter-arrival gap quartiles.

Operands arrive in order:

	[0] events: *[][2]float64
	[1] span: *float64
	[2] originSec: *float64

Yields 20 values:

	[0..6]   muX, muY, beta, alphaXX, alphaXY, alphaYX, alphaYY
	[7..11]  hawkesLL, hawkesPerEventLL, poissonLL, poissonGain, poissonGainPerEvent
	[12..14] selfLL, selfGain, selfGainPerEvent
	[15..19] selfMuX, selfMuY, selfBeta, selfAlphaXX, selfAlphaYY
*/
type Fit struct {
	*core.PrimitiveError
	out [20]float64
}

func NewFit() core.Primitive {
	return &Fit{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Fit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var (
			eventsPtr    *[][2]float64
			spanPtr      *float64
			originSecPtr *float64
		)
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index == 0 {
				eventsPtr = (*[][2]float64)(arriving)
			}

			if index == 1 {
				spanPtr = (*float64)(arriving)
			}

			if index == 2 {
				originSecPtr = (*float64)(arriving)
			}

			index++
		}

		if index < 3 || eventsPtr == nil || spanPtr == nil || originSecPtr == nil {
			op.Error(core.ErrShape)
			return
		}

		events := *eventsPtr
		span := *spanPtr
		originSec := *originSecPtr

		if len(events) < 4 || span <= 0 {
			return
		}

		gaps := make([]float64, 0, len(events)-1)

		for eventIndex := 1; eventIndex < len(events); eventIndex++ {
			gap := events[eventIndex][0] - events[eventIndex-1][0]

			if gap > 0 {
				gaps = append(gaps, gap)
			}
		}

		if len(gaps) < 4 {
			return
		}

		sortedGaps := append([]float64(nil), gaps...)
		sort.Float64s(sortedGaps)
		medianGap := sortedGaps[len(sortedGaps)/2]
		gapQ25 := sortedGaps[len(sortedGaps)/4]
		gapQ75 := sortedGaps[3*len(sortedGaps)/4]

		if gapQ75 <= gapQ25 {
			rootCount := math.Sqrt(float64(len(gaps)))
			gapQ75 = medianGap * (1 + 1/rootCount)
			gapQ25 = medianGap * (1 - 1/rootCount)

			if gapQ25 <= 0 {
				gapQ25 = medianGap / 2
			}
		}

		if medianGap <= 0 || gapQ25 <= 0 || gapQ75 <= 0 {
			return
		}

		betaMin := 1.0 / gapQ75
		betaMax := 1.0 / gapQ25
		minRate := 1.0 / span
		count0 := 0.0
		count1 := 0.0

		for _, event := range events {
			if event[1] == 0 {
				count0++
			}

			if event[1] == 1 {
				count1++
			}
		}

		maxRateX := math.Max(minRate*2, (count0/span)*2)
		maxRateY := math.Max(minRate*2, (count1/span)*2)

		branchFloor := 0.01
		branchCeil := 0.45

		lower := [7]float64{
			math.Log(minRate),
			math.Log(minRate),
			math.Log(betaMin),
			math.Log(branchFloor),
			math.Log(branchFloor),
			math.Log(branchFloor),
			math.Log(branchFloor),
		}

		upper := [7]float64{
			math.Log(maxRateX),
			math.Log(maxRateY),
			math.Log(betaMax),
			math.Log(branchCeil),
			math.Log(branchCeil),
			math.Log(branchCeil),
			math.Log(branchCeil),
		}

		softplus := func(val float64) float64 {
			if val > 20 {
				return val
			}

			if val < -20 {
				return math.Exp(val)
			}

			return math.Log(1 + math.Exp(val))
		}

		inverseSoftplus := func(val float64) float64 {
			if val <= 0 {
				return -20
			}

			if val > 20 {
				return val
			}

			return math.Log(math.Exp(val) - 1)
		}

		softplusDeriv := func(val float64) float64 {
			if val > 20 {
				return 1.0
			}

			if val < -20 {
				return math.Exp(val)
			}

			return 1.0 / (1.0 + math.Exp(-val))
		}

		decode := func(free []float64) [7]float64 {
			var natural [7]float64

			for paramIndex := range 7 {
				paramSpan := upper[paramIndex] - lower[paramIndex]

				if paramSpan <= 0 {
					natural[paramIndex] = math.Exp(lower[paramIndex])
					continue
				}

				lift := softplus(free[paramIndex])
				ratio := lift / (1 + lift)
				natural[paramIndex] = math.Exp(lower[paramIndex] + paramSpan*ratio)
			}

			return natural
		}

		encode := func(logParams [7]float64) []float64 {
			free := make([]float64, 7)

			for paramIndex := range 7 {
				paramSpan := upper[paramIndex] - lower[paramIndex]

				if paramSpan <= 0 {
					free[paramIndex] = 0
					continue
				}

				ratio := (logParams[paramIndex] - lower[paramIndex]) / paramSpan
				ratio = math.Max(1e-9, math.Min(1-1e-9, ratio))
				free[paramIndex] = inverseSoftplus(ratio / (1 - ratio))
			}

			return free
		}

		evalLLAndGrad := func(free []float64, selfOnly bool) (float64, []float64, bool) {
			natural := decode(free)
			muX := natural[0]
			muY := natural[1]
			beta := natural[2]
			p00 := natural[3]
			p01 := natural[4]
			p10 := natural[5]
			p11 := natural[6]

			if selfOnly {
				p01 = 0
				p10 = 0
			}

			alphaXX := p00 * beta
			alphaXY := p01 * beta
			alphaYX := p10 * beta
			alphaYY := p11 * beta

			r0 := 0.0
			r1 := 0.0
			d0 := 0.0
			d1 := 0.0
			lastTime := events[0][0]

			logSum := 0.0
			gradMuX := 0.0
			gradMuY := 0.0
			gradAlphaXX := 0.0
			gradAlphaXY := 0.0
			gradAlphaYX := 0.0
			gradAlphaYY := 0.0
			gradBeta := 0.0

			for eventIndex := 0; eventIndex < len(events); {
				currentTime := events[eventIndex][0]
				age := currentTime - lastTime

				if age > 0 {
					decay := math.Exp(-beta * age)
					d0 = decay * (d0 - age*r0)
					d1 = decay * (d1 - age*r1)
					r0 *= decay
					r1 *= decay
					lastTime = currentTime
				}

				endIndex := eventIndex

				for endIndex < len(events) && events[endIndex][0] == currentTime {
					endIndex++
				}

				if currentTime > originSec {
					for _, ev := range events[eventIndex:endIndex] {
						side := ev[1]

						if side == 0 {
							lambda := muX + alphaXX*r0 + alphaXY*r1

							if lambda <= 0 {
								return 0, nil, false
							}

							logSum += math.Log(lambda)
							invLambda := 1.0 / lambda
							gradMuX += invLambda
							gradAlphaXX += r0 * invLambda
							gradAlphaXY += r1 * invLambda
							gradBeta += (alphaXX*d0 + alphaXY*d1) * invLambda
						}

						if side == 1 {
							lambda := muY + alphaYX*r0 + alphaYY*r1

							if lambda <= 0 {
								return 0, nil, false
							}

							logSum += math.Log(lambda)
							invLambda := 1.0 / lambda
							gradMuY += invLambda
							gradAlphaYX += r0 * invLambda
							gradAlphaYY += r1 * invLambda
							gradBeta += (alphaYX*d0 + alphaYY*d1) * invLambda
						}
					}
				}

				for _, ev := range events[eventIndex:endIndex] {
					side := ev[1]

					if side == 0 {
						r0 += 1.0
					}

					if side == 1 {
						r1 += 1.0
					}
				}

				eventIndex = endIndex
			}

			horizonSec := originSec + span
			buySupport := 0.0
			sellSupport := 0.0
			dBuySupport := 0.0
			dSellSupport := 0.0

			for _, ev := range events {
				eventTime := ev[0]

				if eventTime <= horizonSec {
					lowerAge := originSec - eventTime

					if lowerAge < 0 {
						lowerAge = 0
					}

					upperAge := horizonSec - eventTime

					if upperAge > lowerAge {
						expLower := math.Exp(-beta * lowerAge)
						expUpper := math.Exp(-beta * upperAge)
						supportDiff := expLower - expUpper
						dSupportDiff := -lowerAge*expLower + upperAge*expUpper

						if ev[1] == 0 {
							buySupport += supportDiff
							dBuySupport += dSupportDiff
						}

						if ev[1] == 1 {
							sellSupport += supportDiff
							dSellSupport += dSupportDiff
						}
					}
				}
			}

			invBeta := 1.0 / beta
			invBetaSq := invBeta * invBeta

			comp0 := muX*span + alphaXX*invBeta*buySupport + alphaXY*invBeta*sellSupport
			comp1 := muY*span + alphaYX*invBeta*buySupport + alphaYY*invBeta*sellSupport
			compensatorTotal := comp0 + comp1

			logLikelihood := logSum - compensatorTotal

			gradMuX -= span
			gradMuY -= span
			gradAlphaXX -= invBeta * buySupport
			gradAlphaXY -= invBeta * sellSupport
			gradAlphaYX -= invBeta * buySupport
			gradAlphaYY -= invBeta * sellSupport

			dComp0dBeta := -alphaXX*invBetaSq*buySupport + alphaXX*invBeta*dBuySupport -
				alphaXY*invBetaSq*sellSupport + alphaXY*invBeta*dSellSupport
			dComp1dBeta := -alphaYX*invBetaSq*buySupport + alphaYX*invBeta*dBuySupport -
				alphaYY*invBetaSq*sellSupport + alphaYY*invBeta*dSellSupport

			gradBeta += (dComp0dBeta + dComp1dBeta)

			gradLog := [7]float64{
				gradMuX * muX,
				gradMuY * muY,
				gradBeta*beta + gradAlphaXX*alphaXX + gradAlphaXY*alphaXY + gradAlphaYX*alphaYX + gradAlphaYY*alphaYY,
				gradAlphaXX * alphaXX,
				gradAlphaXY * alphaXY,
				gradAlphaYX * alphaYX,
				gradAlphaYY * alphaYY,
			}

			if selfOnly {
				gradLog[4] = 0
				gradLog[5] = 0
			}

			gradFree := make([]float64, 7)

			for paramIndex := range 7 {
				paramSpan := upper[paramIndex] - lower[paramIndex]

				if paramSpan <= 0 {
					gradFree[paramIndex] = 0
					continue
				}

				lift := softplus(free[paramIndex])
				dRatio := (softplusDeriv(free[paramIndex])) / ((1 + lift) * (1 + lift))
				dLogParam := paramSpan * dRatio
				gradFree[paramIndex] = -gradLog[paramIndex] * dLogParam
			}

			return logLikelihood, gradFree, true
		}

		optimizeWithSeeds := func(selfOnly bool) (float64, [7]float64, bool) {
			muXInit := math.Max(minRate, count0/span*0.5)
			muYInit := math.Max(minRate, count1/span*0.5)
			betaInit := 1.0 / medianGap

			seed0 := encode([7]float64{
				math.Log(muXInit),
				math.Log(muYInit),
				math.Log(betaInit),
				math.Log(0.15),
				math.Log(0.10),
				math.Log(0.10),
				math.Log(0.15),
			})

			seed1 := encode([7]float64{
				math.Log(muXInit * 1.5),
				math.Log(muYInit * 1.5),
				math.Log(betaMin * 1.2),
				math.Log(0.05),
				math.Log(0.02),
				math.Log(0.02),
				math.Log(0.05),
			})

			seed2 := encode([7]float64{
				math.Log(muXInit * 0.7),
				math.Log(muYInit * 0.7),
				math.Log(betaMax * 0.8),
				math.Log(0.30),
				math.Log(0.20),
				math.Log(0.20),
				math.Log(0.30),
			})

			bestNegLL := math.Inf(1)
			var bestFree []float64
			found := false

			for _, seed := range [][]float64{seed0, seed1, seed2} {
				prob := optimize.Problem{
					Func: func(x []float64) float64 {
						llVal, _, valid := evalLLAndGrad(x, selfOnly)

						if !valid {
							return 1e12
						}

						return -llVal
					},
					Grad: func(grad, x []float64) {
						_, gFree, valid := evalLLAndGrad(x, selfOnly)

						if !valid {
							for indexGrad := range grad {
								grad[indexGrad] = 0
							}
							return
						}

						copy(grad, gFree)
					},
				}

				settings := &optimize.Settings{
					MajorIterations:   30,
					GradientThreshold: 1e-4,
				}

				res, err := optimize.Minimize(prob, append([]float64(nil), seed...), settings, &optimize.LBFGS{})

				if err == nil && res != nil && res.F < bestNegLL {
					bestNegLL = res.F
					bestFree = res.X
					found = true
				}
			}

			if !found {
				return 0, [7]float64{}, false
			}

			natural := decode(bestFree)

			if selfOnly {
				natural[4] = 0
				natural[5] = 0
			}

			return -bestNegLL, natural, true
		}

		bestLL, natural, success := optimizeWithSeeds(false)

		if !success {
			return
		}

		muXFit := natural[0]
		muYFit := natural[1]
		betaFit := natural[2]
		alphaXXFit := natural[3] * betaFit
		alphaXYFit := natural[4] * betaFit
		alphaYXFit := natural[5] * betaFit
		alphaYYFit := natural[6] * betaFit

		markedCount := float64(len(events))
		poissonLL := count0*math.Log(muXFit) + count1*math.Log(muYFit) - (muXFit+muYFit)*span

		selfLL, selfNatural, selfSuccess := optimizeWithSeeds(true)
		selfMuX := 0.0
		selfMuY := 0.0
		selfBeta := 0.0
		selfAlphaXX := 0.0
		selfAlphaYY := 0.0
		selfGain := 0.0
		selfGainPerEvent := 0.0

		if selfSuccess {
			selfMuX = selfNatural[0]
			selfMuY = selfNatural[1]
			selfBeta = selfNatural[2]
			selfAlphaXX = selfNatural[3] * selfBeta
			selfAlphaYY = selfNatural[6] * selfBeta
			selfGain = bestLL - selfLL
			selfGainPerEvent = selfGain / markedCount
		}

		clear(op.out[:])

		op.out[0] = muXFit
		op.out[1] = muYFit
		op.out[2] = betaFit
		op.out[3] = alphaXXFit
		op.out[4] = alphaXYFit
		op.out[5] = alphaYXFit
		op.out[6] = alphaYYFit
		op.out[7] = bestLL
		op.out[8] = bestLL / markedCount
		op.out[9] = poissonLL
		op.out[10] = bestLL - poissonLL
		op.out[11] = (bestLL - poissonLL) / markedCount
		op.out[12] = selfLL
		op.out[13] = selfGain
		op.out[14] = selfGainPerEvent
		op.out[15] = selfMuX
		op.out[16] = selfMuY
		op.out[17] = selfBeta
		op.out[18] = selfAlphaXX
		op.out[19] = selfAlphaYY

		for outIndex := range op.out {
			val := op.out[outIndex]

			if !yield(unsafe.Pointer(&val)) {
				return
			}
		}
	}
}
