package hawkes

import (
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)



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
Process evaluates an arriving marked point process [timestampNano, mark]
into its empirical arrival state and fitted Hawkes decomposition.
Mark > 0 represents buy/A, mark <= 0 represents sell/B.
No structs, pure Value closure.
*/
type Process types.Value[[2]float64, Reading]

func NewProcess(params ...types.Float) Process {
	state := &path{samples: make([]sample, 0)}

	return func(event [2]float64) Reading {
		atNano := int64(event[0])
		markRaw := event[1]

		if atNano <= 0 {
			return Reading{}
		}

		at := time.Unix(0, atNano)

		if state.hasLast && at.Before(state.lastAt) {
			return Reading{}
		}

		mark := -core.Unit
		if markRaw > 0 {
			mark = core.Unit
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

		atSec := float64(atNano) * 1e-9
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
			evaluateReading(&reading, state, buyArrivals, sellArrivals, atSec)
		}

		state.lastAt = at
		state.hasLast = true
		state.remember(at, atSec, mark)
		state.refit(atSec)

		return reading
	}
}

func evaluateReading(
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
