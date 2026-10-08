package hawkes

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Parameters owns Hawkes model parameters, timescales, branching ratios, spectral
radius, and nested likelihood metrics as a Primitive.

It receives the 20 values yielded by Fit:

	[0]  muX
	[1]  muY
	[2]  beta
	[3]  alphaXX
	[4]  alphaXY
	[5]  alphaYX
	[6]  alphaYY
	[7]  hawkesLogLikelihood
	[8]  hawkesPerEventLogLikelihood
	[9]  poissonLogLikelihood
	[10] poissonGain
	[11] poissonGainPerEvent
	[12] selfLogLikelihood
	[13] selfGain
	[14] selfGainPerEvent
	[15] selfMuX
	[16] selfMuY
	[17] selfBeta
	[18] selfAlphaXX
	[19] selfAlphaYY

It yields 29 derived parameter and model-comparison metrics in order:

	[0..3]   alphaXX, alphaXY, alphaYX, alphaYY
	[4..8]   beta, beta, beta, beta, beta
	[9..13]  timescale, timescale, timescale, timescale, timescale
	[14..17] p00, p01, p10, p11 (offspring)
	[18]     spectralRadius
	[19..20] desc0, desc1 (expected descendants)
	[21..28] hawkesLL, hawkesPerEventLL, poissonLL, poissonGain, poissonGainPerEvent,
	         selfLL, selfGain, selfGainPerEvent
*/
type Parameters struct {
	*core.PrimitiveError
	ready       bool
	muX         float64
	muY         float64
	beta        float64
	alphaXX     float64
	alphaXY     float64
	alphaYX     float64
	alphaYY     float64
	selfReady   bool
	selfMuX     float64
	selfMuY     float64
	selfBeta    float64
	selfAlphaXX float64
	selfAlphaYY float64
	out         [29]float64
}

func NewParameters() core.Primitive {
	return &Parameters{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Parameters) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [20]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 20 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 20 {
			op.Error(core.ErrShape)
			return
		}

		op.muX = values[0]
		op.muY = values[1]
		op.beta = values[2]
		op.alphaXX = values[3]
		op.alphaXY = values[4]
		op.alphaYX = values[5]
		op.alphaYY = values[6]
		op.ready = true

		op.selfMuX = values[15]
		op.selfMuY = values[16]
		op.selfBeta = values[17]
		op.selfAlphaXX = values[18]
		op.selfAlphaYY = values[19]
		op.selfReady = true

		clear(op.out[:])

		op.out[0] = op.alphaXX
		op.out[1] = op.alphaXY
		op.out[2] = op.alphaYX
		op.out[3] = op.alphaYY

		timescale := 0.0

		if op.beta > 0 {
			timescale = 1.0 / op.beta
			op.out[4] = op.beta
			op.out[5] = op.beta
			op.out[6] = op.beta
			op.out[7] = op.beta
			op.out[8] = op.beta
			op.out[9] = timescale
			op.out[10] = timescale
			op.out[11] = timescale
			op.out[12] = timescale
			op.out[13] = timescale
		}

		p00 := 0.0
		p01 := 0.0
		p10 := 0.0
		p11 := 0.0

		if op.beta > 0 {
			p00 = op.alphaXX / op.beta
			p01 = op.alphaXY / op.beta
			p10 = op.alphaYX / op.beta
			p11 = op.alphaYY / op.beta
		}

		op.out[14] = p00
		op.out[15] = p01
		op.out[16] = p10
		op.out[17] = p11

		trace := p00 + p11
		det := p00*p11 - p01*p10
		discriminant := trace*trace - 4*det
		spectralRadius := 0.0

		if discriminant >= 0 {
			spectralRadius = (math.Abs(trace) + math.Sqrt(discriminant)) / 2
		}

		if discriminant < 0 {
			spectralRadius = math.Sqrt(det)
		}

		op.out[18] = spectralRadius

		detIdentity := (1-p00)*(1-p11) - p01*p10

		if detIdentity > 0 && p00 < 1 && p11 < 1 {
			op.out[19] = (1 - p11 + p10) / detIdentity
			op.out[20] = (1 - p00 + p01) / detIdentity
		}

		op.out[21] = values[7]
		op.out[22] = values[8]
		op.out[23] = values[9]
		op.out[24] = values[10]
		op.out[25] = values[11]
		op.out[26] = values[12]
		op.out[27] = values[13]
		op.out[28] = values[14]

		for itemIndex := range op.out {
			val := op.out[itemIndex]

			if !yield(unsafe.Pointer(&val)) {
				return
			}
		}
	}
}
