package hawkes

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Hawkes implements a streaming bivariate mutually exciting point process as
a Primitive. It estimates joint background rates, cross-excitation amplitudes,
decay timescales, branching ratios, compensator innovations, and empirical SNR
directly from honest market arrival dynamics.

Operands arrive in order: [mark, atSec] (or [buy, sell, atSec]).
Yields 62 values matching outputKeys, plus fromSec.
*/
type Hawkes struct {
	*core.PrimitiveError
	hasLast       bool
	lastAt        float64
	originSec     float64
	stream0       []float64
	stream1       []float64
	events        [][2]float64
	modelReady    bool
	muX           float64
	muY           float64
	alphaXX       float64
	alphaXY       float64
	alphaYX       float64
	alphaYY       float64
	beta          float64
	selfOnlyReady bool
	selfMuX       float64
	selfMuY       float64
	selfAlphaXX   float64
	selfAlphaYY   float64
	selfBeta      float64
	paramMetrics  [29]float64
	hasSNR        bool
	snr           float64
	fit           core.Primitive
	parameters    core.Primitive
	cached        [62]float64
}

func NewHawkes() core.Primitive {
	return &Hawkes{
		PrimitiveError: core.NewPrimitiveError(),
		fit:            NewFit(),
		parameters:     NewParameters(),
	}
}

func (op *Hawkes) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 3 {
				values[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 2 {
			op.Error(core.ErrShape)
			return
		}

		var mark, atSec float64

		if idx == 2 {
			mark = values[0]
			atSec = values[1]
		}

		if idx == 3 {
			buy := values[0]
			sell := values[1]
			atSec = values[2]

			if buy == 1 && sell == 0 {
				mark = 1.0
			}

			if buy == 0 && sell == 1 {
				mark = -1.0
			}

			if (buy != 1 && buy != 0) || (sell != 1 && sell != 0) || (buy == sell) {
				op.Error(fmt.Errorf("%w: hawkes: trade side carries no excitation mark", core.ErrDomain))
				return
			}
		}

		if op.hasLast && atSec < op.lastAt {
			op.Error(fmt.Errorf("%w: hawkes: regressing event time", core.ErrDomain))
			return
		}

		op.lastAt = atSec
		op.hasLast = true

		if len(op.events) == 0 {
			op.originSec = atSec
		}

		fromSec := op.originSec
		span := atSec - fromSec

		count0 := float64(len(op.stream0))
		count1 := float64(len(op.stream1))

		if mark > 0 {
			count0++
		}

		if mark <= 0 {
			count1++
		}

		totalCount := count0 + count1
		frac0 := count0 / totalCount
		frac1 := count1 / totalCount

		var rate0, rate1, rate float64

		if span > 0 {
			rate0 = count0 / span
			rate1 = count1 / span
			rate = totalCount / span
		}

		clear(op.cached[:])
		op.cached[0] = totalCount
		op.cached[1] = count0
		op.cached[2] = count1
		op.cached[3] = frac0
		op.cached[4] = frac1
		op.cached[5] = rate0
		op.cached[6] = rate1
		op.cached[7] = rate

		if op.modelReady {
			muX := op.muX
			muY := op.muY
			alphaXX := op.alphaXX
			alphaXY := op.alphaXY
			alphaYX := op.alphaYX
			alphaYY := op.alphaYY
			beta := op.beta

			sum0 := 0.0

			for _, eventTime := range op.stream0 {
				if eventTime <= atSec {
					age := atSec - eventTime

					if age > 0 {
						sum0 += math.Exp(-beta * age)
					}
				}
			}

			sum1 := 0.0

			for _, eventTime := range op.stream1 {
				if eventTime <= atSec {
					age := atSec - eventTime

					if age > 0 {
						sum1 += math.Exp(-beta * age)
					}
				}
			}

			lambda0 := muX + alphaXX*sum0 + alphaXY*sum1
			lambda1 := muY + alphaYX*sum0 + alphaYY*sum1
			excess0 := lambda0 - muX
			excess1 := lambda1 - muY

			op.cached[8] = lambda0
			op.cached[9] = lambda1
			op.cached[10] = lambda0 + lambda1
			op.cached[11] = muX
			op.cached[12] = muY
			op.cached[13] = muX + muY
			op.cached[14] = excess0
			op.cached[15] = excess1

			if lambda0 > 0 {
				op.cached[16] = excess0 / lambda0
			}

			if lambda1 > 0 {
				op.cached[17] = excess1 / lambda1
			}

			// Parameter block from parameters primitive:
			// op.paramMetrics indices:
			// 0..3: alphaXX, alphaXY, alphaYX, alphaYY
			// 4..8: beta, beta, beta, beta, beta
			// 9..13: timescale, timescale, timescale, timescale, timescale
			// 14..17: p00, p01, p10, p11
			// 18: spectralRadius
			// 19..20: desc0, desc1
			// 21..28: hawkesLL, hawkesPerEventLL, poissonLL, poissonGain, poissonGainPerEvent, selfLL, selfGain, selfGainPerEvent
			copy(op.cached[18:39], op.paramMetrics[0:21])
			copy(op.cached[39:47], op.paramMetrics[21:29])

			if span > 0 {
				buySupport := 0.0

				for _, eventTime := range op.stream0 {
					if eventTime <= atSec {
						lowerAge := op.originSec - eventTime

						if lowerAge < 0 {
							lowerAge = 0
						}

						upperAge := atSec - eventTime

						if upperAge > lowerAge {
							buySupport += math.Exp(-beta*lowerAge) - math.Exp(-beta*upperAge)
						}
					}
				}

				sellSupport := 0.0

				for _, eventTime := range op.stream1 {
					if eventTime <= atSec {
						lowerAge := op.originSec - eventTime

						if lowerAge < 0 {
							lowerAge = 0
						}

						upperAge := atSec - eventTime

						if upperAge > lowerAge {
							sellSupport += math.Exp(-beta*lowerAge) - math.Exp(-beta*upperAge)
						}
					}
				}

				compBuy := muX*span + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
				compSell := muY*span + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport
				totalCompensator := compBuy + compSell

				priorCountBuy := float64(len(op.stream0))
				priorCountSell := float64(len(op.stream1))
				innoBuy := priorCountBuy - compBuy
				innoSell := priorCountSell - compSell

				op.cached[47] = compBuy
				op.cached[48] = compSell
				op.cached[49] = innoBuy
				op.cached[50] = innoSell

				if compBuy > 0 {
					op.cached[51] = innoBuy / math.Sqrt(compBuy)
				}

				if compSell > 0 {
					op.cached[52] = innoSell / math.Sqrt(compSell)
				}

				excessBuyMass := compBuy - muX*span
				excessSellMass := compSell - muY*span

				op.cached[53] = excessBuyMass
				op.cached[54] = excessSellMass

				if compBuy > 0 {
					op.cached[55] = excessBuyMass / compBuy
				}

				if compSell > 0 {
					op.cached[56] = excessSellMass / compSell
				}

				if totalCompensator > 0 {
					op.cached[57] = (excessBuyMass + excessSellMass) / totalCompensator
				}

				snrSum := 0.0
				snrSides := 0

				if compBuy > 0 {
					snrSum += (excessBuyMass * excessBuyMass) / compBuy
					snrSides++
				}

				if compSell > 0 {
					snrSum += (excessSellMass * excessSellMass) / compSell
					snrSides++
				}

				if snrSides > 0 {
					op.snr = snrSum / float64(snrSides)
					op.hasSNR = true
					op.cached[58] = op.snr
				}
			}
		}

		op.cached[61] = fromSec

		if mark > 0 {
			op.stream0 = append(op.stream0, atSec)
			op.events = append(op.events, [2]float64{atSec, 0})
		}

		if mark <= 0 {
			op.stream1 = append(op.stream1, atSec)
			op.events = append(op.events, [2]float64{atSec, 1})
		}

		if len(op.events) >= 10 && span > 0 {
			eventsClone := slices.Clone(op.events)
			spanVal := span
			originVal := op.originSec

			fitIn := data.NewValue(
				unsafe.Pointer(&eventsClone),
				unsafe.Pointer(&spanVal),
				unsafe.Pointer(&originVal),
			)

			paramIndex := 0

			for paramPtr := range op.parameters.Next(op.fit.Next(fitIn.Next(nil))) {
				if paramPtr != nil && paramIndex < len(op.paramMetrics) {
					op.paramMetrics[paramIndex] = *(*float64)(paramPtr)
					paramIndex++
				}
			}

			if paramIndex == len(op.paramMetrics) {
				p := op.parameters.(*Parameters)
				op.muX = p.muX
				op.muY = p.muY
				op.beta = p.beta
				op.alphaXX = p.alphaXX
				op.alphaXY = p.alphaXY
				op.alphaYX = p.alphaYX
				op.alphaYY = p.alphaYY
				op.modelReady = p.ready

				op.selfMuX = p.selfMuX
				op.selfMuY = p.selfMuY
				op.selfBeta = p.selfBeta
				op.selfAlphaXX = p.selfAlphaXX
				op.selfAlphaYY = p.selfAlphaYY
				op.selfOnlyReady = p.selfReady
			}
		}

		for _, val := range op.cached {
			v := val

			if !yield(unsafe.Pointer(&v)) {
				return
			}
		}
	}
}
