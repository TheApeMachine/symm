package correlation

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LeadLag composes the supplied estimator around an exact discrete search. Each
arrival is *[2][][2]float64{leftPrices, rightPrices}; it yields *[2][]float64
where [0] is one summary row
{correlation, covariance, support, leftEnergy, rightEnergy, defined,
index, lagIndex, x, y, leads, shapeDefined, contemporaneous, searchCount,
spacing, span, observations, searchScale, absoluteGain, lagFraction,
prominence, curvature}
and [1] is the flattened profile of 10-float candidates.
*/
type LeadLag struct {
	*core.PrimitiveError
	estimator    core.Primitive
	leftReturns  core.Primitive
	rightReturns core.Primitive
	query        [3][]float64
	spacings     []float64
	candidates   []float64
	nonzero      []int
	summary      []float64
	out          [2][]float64
}

func NewLeadLag(estimator core.Primitive) core.Primitive {
	return &LeadLag{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		leftReturns:    NewReturns(),
		rightReturns:   NewReturns(),
		summary:        make([]float64, 22),
	}
}

func (op *LeadLag) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][][2]float64)(arriving)
			clear(op.summary)
			op.candidates = op.candidates[:0]
			op.nonzero = op.nonzero[:0]
			op.out[0] = op.summary
			op.out[1] = op.candidates

			observations := math.Min(float64(len(input[0])), float64(len(input[1])))

			if observations <= 2 {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			count := len(input[0]) - 1

			if count <= 0 {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if cap(op.spacings) < count {
				op.spacings = make([]float64, count)
			} else {
				op.spacings = op.spacings[:count]
			}

			for i := 1; i < len(input[0]); i++ {
				op.spacings[i-1] = input[0][i][0] - input[0][i-1][0]
			}

			slices.Sort(op.spacings)
			leftSpacing := (op.spacings[(count-1)/2] + op.spacings[count/2]) * 0.5

			count = len(input[1]) - 1

			if count <= 0 {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if cap(op.spacings) < count {
				op.spacings = make([]float64, count)
			} else {
				op.spacings = op.spacings[:count]
			}

			for i := 1; i < len(input[1]); i++ {
				op.spacings[i-1] = input[1][i][0] - input[1][i-1][0]
			}

			slices.Sort(op.spacings)
			rightSpacing := (op.spacings[(count-1)/2] + op.spacings[count/2]) * 0.5
			spacing := math.Min(leftSpacing, rightSpacing)

			if spacing <= 0 {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			span := observations - 2
			var left, right [2][]float64

			for pointer := range op.leftReturns.Next(data.NewValue(input[0])) {
				left = *(*[2][]float64)(pointer)
			}

			if err := op.leftReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			for pointer := range op.rightReturns.Next(data.NewValue(input[1])) {
				right = *(*[2][]float64)(pointer)
			}

			if err := op.rightReturns.Error(); err != nil {
				op.Error(err)
				return
			}

			leftEnergy, rightEnergy := 0.0, 0.0

			if len(left[1]) > 0 {
				leftEnergy = left[1][0]
			}

			if len(right[1]) > 0 {
				rightEnergy = right[1][0]
			}

			op.query[0] = left[0]
			op.query[1] = right[0]
			op.query[2] = []float64{leftEnergy, rightEnergy, 0}

			limit := int(span*2 + 1)

			if cap(op.candidates) < limit*10 {
				op.candidates = make([]float64, 0, limit*10)
			}

			for index := 0; index < limit; index++ {
				lagIndex := float64(index) - span
				lag := lagIndex * spacing
				op.query[2][2] = lag

				var reading [6]float64

				for pointer := range op.estimator.Next(data.NewValue(op.query)) {
					reading = *(*[6]float64)(pointer)
				}

				if err := op.estimator.Error(); err != nil {
					op.Error(err)
					return
				}

				start := len(op.candidates)
				op.candidates = append(op.candidates,
					reading[0], reading[1], reading[2], reading[3], reading[4], reading[5],
					float64(index), lagIndex, lag*1e-9, reading[0],
				)

				if reading[5] == 1 && lag != 0 {
					op.nonzero = append(op.nonzero, start)
				}
			}

			if len(op.nonzero) == 0 {
				op.out[1] = op.candidates

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			best := op.nonzero[0]
			maxMag := math.Abs(op.candidates[best+9])

			for _, start := range op.nonzero[1:] {
				mag := math.Abs(op.candidates[start+9])

				if mag > maxMag {
					maxMag = mag
					best = start
				}
			}

			zero := int(span) * 10
			searchScale := math.Sqrt(2.0 * math.Log(float64(len(op.nonzero)+1)) / (observations - 1))
			absoluteGain := math.Abs(op.candidates[best]) - math.Abs(op.candidates[zero])
			leads := 0.0

			if math.Abs(op.candidates[best]) > searchScale && absoluteGain > 0 {
				leads = 1
			}

			lagFraction := 0.0

			if leads == 1 {
				lagFraction = math.Abs(op.candidates[best+7]) / span
			}

			shapeIdx := int(op.candidates[best+6])
			shapeDefined := 0.0
			prominence := 0.0
			curvature := 0.0

			if op.candidates[best+6] > 0 && op.candidates[best+6] < span*2 && shapeIdx > 0 && shapeIdx < limit-1 {
				lower := shapeIdx*10 - 10
				upper := shapeIdx*10 + 10

				if op.candidates[lower+5] == 1 && op.candidates[upper+5] == 1 {
					leftVal := math.Abs(op.candidates[lower+9])
					centerVal := math.Abs(op.candidates[shapeIdx*10+9])
					rightVal := math.Abs(op.candidates[upper+9])
					diff := 2.0*centerVal - leftVal - rightVal
					seconds := spacing * 1e-9
					shapeDefined = 1
					prominence = diff / 2.0
					curvature = diff / (seconds * seconds)
				}
			}

			op.summary = op.summary[:22]
			op.summary[0] = op.candidates[best]
			op.summary[1] = op.candidates[best+1]
			op.summary[2] = op.candidates[best+2]
			op.summary[3] = op.candidates[best+3]
			op.summary[4] = op.candidates[best+4]
			op.summary[5] = 1
			op.summary[6] = op.candidates[best+6]
			op.summary[7] = op.candidates[best+7]
			op.summary[8] = op.candidates[best+8]
			op.summary[9] = op.candidates[best+9]
			op.summary[10] = leads
			op.summary[11] = shapeDefined
			op.summary[12] = op.candidates[zero]
			op.summary[13] = float64(len(op.nonzero))
			op.summary[14] = spacing
			op.summary[15] = span
			op.summary[16] = observations
			op.summary[17] = searchScale
			op.summary[18] = absoluteGain
			op.summary[19] = lagFraction
			op.summary[20] = prominence
			op.summary[21] = curvature

			op.out[0] = op.summary
			op.out[1] = op.candidates

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
