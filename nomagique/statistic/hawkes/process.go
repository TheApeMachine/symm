package hawkes

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Event is one marked arrival.
*/
type Event struct {
	Symbol string
	Side   string
	At     int64
}

/*
Reading is the empirical arrival state and, when a model exists, the
pre-arrival Hawkes decomposition.
*/
type Reading struct {
	EventCount, BuyCount, SellCount                    float64
	BuyFraction, SellFraction                          float64
	ArrivalRate, BuyRate, SellRate                     float64
	HasRates                                           bool
	HasFit                                             bool
	LambdaBuy, LambdaSell, Lambda                      float64
	MuBuy, MuSell, Mu                                  float64
	ExcessBuy, ExcessSell                              float64
	ExcitationBuyFrac, ExcitationSellFrac              float64
	AlphaBB, AlphaBS, AlphaSB, AlphaSS                 float64
	Beta, Timescale                                    float64
	OffspringBB, OffspringBS, OffspringSB, OffspringSS float64
	SpectralRadius                                     float64
	DescendantsBuy, DescendantsSell                    float64
	HasDescendants                                     bool
	CompensatorBuy, CompensatorSell                    float64
	InnovationBuy, InnovationSell                      float64
	SNR                                                float64
	HasSNR                                             bool
}

/*
Process owns per-symbol arrival history, empirical counts, and the fitted
bivariate Hawkes evaluation.
*/
type Process struct {
	*core.PrimitiveError

	history *paths
	out     Reading
}

func NewProcess() *Process {
	return &Process{PrimitiveError: core.NewPrimitiveError(), history: newPaths()}
}

func (process *Process) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			event := *(*Event)(arriving)

			if event.Side != "buy" && event.Side != "sell" {
				continue
			}

			state := process.history.at(event.Symbol)
			at := time.Unix(0, event.At)

			if state.hasLast && at.Before(state.lastAt) {
				continue
			}

			mark := -1.0

			if event.Side == "buy" {
				mark = 1.0
			}

			buyArrivals, sellArrivals := state.sides()
			countBuy := float64(len(buyArrivals))
			countSell := float64(len(sellArrivals))

			if mark > 0 {
				countBuy++
			}

			if mark <= 0 {
				countSell++
			}

			count := countBuy + countSell
			reading := Reading{
				EventCount:   count,
				BuyCount:     countBuy,
				SellCount:    countSell,
				BuyFraction:  countBuy / count,
				SellFraction: countSell / count,
			}

			from := at

			if len(state.samples) > 0 {
				from = state.origin()
			}

			atSec := float64(event.At) * 1e-9
			fromSec := float64(from.UnixNano()) * 1e-9
			span := atSec - fromSec

			if span > 0 {
				reading.HasRates = true
				reading.BuyRate = countBuy / span
				reading.SellRate = countSell / span
				reading.ArrivalRate = count / span
			}

			if state.modelReady {
				reading.HasFit = true
				process.evaluate(&reading, state, buyArrivals, sellArrivals, atSec)
			}

			state.lastAt = at
			state.hasLast = true
			state.remember(at, atSec, mark)
			state.refit(atSec)
			process.out = reading

			if !yield(unsafe.Pointer(&process.out)) {
				return
			}
		}
	}
}

func (process *Process) evaluate(
	reading *Reading,
	state *path,
	buyArrivals, sellArrivals []float64,
	atSec float64,
) {
	model := state.model
	lambdaBuy := intensityAt(buyArrivals, sellArrivals, atSec, model.muX, model.alphaXX, model.alphaXY, model.beta)
	lambdaSell := intensityAt(buyArrivals, sellArrivals, atSec, model.muY, model.alphaYX, model.alphaYY, model.beta)
	reading.LambdaBuy = lambdaBuy
	reading.LambdaSell = lambdaSell
	reading.Lambda = lambdaBuy + lambdaSell
	reading.MuBuy = model.muX
	reading.MuSell = model.muY
	reading.Mu = model.muX + model.muY
	reading.ExcessBuy = lambdaBuy - model.muX
	reading.ExcessSell = lambdaSell - model.muY

	if lambdaBuy > 0 {
		reading.ExcitationBuyFrac = reading.ExcessBuy / lambdaBuy
	}

	if lambdaSell > 0 {
		reading.ExcitationSellFrac = reading.ExcessSell / lambdaSell
	}

	reading.AlphaBB = model.alphaXX
	reading.AlphaBS = model.alphaXY
	reading.AlphaSB = model.alphaYX
	reading.AlphaSS = model.alphaYY

	if model.beta > 0 {
		reading.Beta = model.beta
		reading.Timescale = 1 / model.beta
	}

	matrix := branchingMatrix(model.alphaXX, model.alphaXY, model.alphaYX, model.alphaYY, model.beta)
	reading.OffspringBB = matrix[0][0]
	reading.OffspringBS = matrix[0][1]
	reading.OffspringSB = matrix[1][0]
	reading.OffspringSS = matrix[1][1]
	reading.SpectralRadius = spectralRadius(matrix)
	buyParent, sellParent, hasDesc := totalDescendants(
		model.alphaXX, model.alphaXY, model.alphaYX, model.alphaYY, model.beta,
	)

	if hasDesc {
		reading.HasDescendants = true
		reading.DescendantsBuy = buyParent
		reading.DescendantsSell = sellParent
	}

	streamPrior := newArrivalStream(buyArrivals, sellArrivals)
	spanPrior := streamPrior.span(atSec)

	if spanPrior <= 0 || model.beta <= 0 {
		return
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, model.beta)
	compBuy := model.muX*spanPrior + (model.alphaXX/model.beta)*buySupport + (model.alphaXY/model.beta)*sellSupport
	compSell := model.muY*spanPrior + (model.alphaYX/model.beta)*buySupport + (model.alphaYY/model.beta)*sellSupport
	reading.CompensatorBuy = compBuy
	reading.CompensatorSell = compSell
	reading.InnovationBuy = float64(len(buyArrivals)) - compBuy
	reading.InnovationSell = float64(len(sellArrivals)) - compSell
	excessBuyMass := compBuy - model.muX*spanPrior
	excessSellMass := compSell - model.muY*spanPrior
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
		reading.SNR = snrSum / float64(snrSides)
		reading.HasSNR = true
		state.snr = reading.SNR
		state.hasSNR = true
	}
}
