package correlation

import (
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
LeadLagReading is one lead/lag search. Empty searches are explicit undefined
records.
*/
type LeadLagReading struct {
	LagCandidate
	Profile         []LagCandidate
	Defined         bool
	Leads           bool
	ShapeDefined    bool
	Contemporaneous float64
	SearchCount     float64
	Spacing         float64
	Span            float64
	Observations    float64
	SearchScale     float64
	AbsoluteGain    float64
	LagFraction     float64
	Prominence      float64
	Curvature       float64
}

/*
LeadLag composes the supplied estimator around an exact discrete search.
*/
type LeadLag struct {
	*core.PrimitiveError

	estimator   core.Primitive
	pathReturns core.Primitive
	out         LeadLagReading
}

/*
NewLeadLag creates a new LeadLag primitive over the supplied estimator.
*/
func NewLeadLag(estimator core.Primitive) *LeadLag {
	return &LeadLag{PrimitiveError: core.NewPrimitiveError(), estimator: estimator, pathReturns: temporal.NewPathReturns()}
}

func (leadLag *LeadLag) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if leadLag.pathReturns != nil {
				if err := leadLag.pathReturns.Error(); err != nil {
					leadLag.Error(err)
				}
			}
		}()
		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			undefined := LeadLagReading{Profile: []LagCandidate{}}
			observations := math.Min(float64(len(input.Left)), float64(len(input.Right)))

			if observations <= 2 {
				leadLag.out = undefined

				if !yield(unsafe.Pointer(&leadLag.out)) {
					return
				}
				continue
			}

			leftSpacing := medianSpacing(input.Left)
			rightSpacing := medianSpacing(input.Right)
			spacing := math.Min(leftSpacing, rightSpacing)

			if spacing <= 0 {
				leadLag.out = undefined

				if !yield(unsafe.Pointer(&leadLag.out)) {
					return
				}
				continue
			}

			span := observations - 2
			left, err := decodePath(leadLag.pathReturns, input.Left)

			if err != nil {
				leadLag.Error(err)
				return
			}

			right, err := decodePath(leadLag.pathReturns, input.Right)

			if err != nil {
				leadLag.Error(err)
				return
			}

			limit := int(span*2 + 1)
			candidates := make([]LagCandidate, 0, limit)
			nonzero := make([]LagCandidate, 0, limit)

			for index := 0; index < limit; index++ {
				lagIndex := float64(index) - span
				lag := int64(lagIndex * spacing)
				reading, err := estimateAt(leadLag.estimator, left, right, lag)

				if err != nil {
					leadLag.Error(err)
					return
				}

				current := LagCandidate{
					LagEstimate: reading,
					Index:       float64(index),
					LagIndex:    lagIndex,
					X:           float64(lag) * 1e-9,
					Y:           reading.Correlation,
				}
				candidates = append(candidates, current)

				if current.Defined && current.X != 0 {
					nonzero = append(nonzero, current)
				}
			}

			if len(nonzero) == 0 {
				leadLag.out = undefined

				if !yield(unsafe.Pointer(&leadLag.out)) {
					return
				}
				continue
			}

			bestIdx := 0
			maxMag := math.Abs(nonzero[0].Y)

			for i := 1; i < len(nonzero); i++ {
				mag := math.Abs(nonzero[i].Y)

				if mag > maxMag {
					maxMag = mag
					bestIdx = i
				}
			}

			selected := nonzero[bestIdx]
			zero := candidates[int(span)]
			searchScale := math.Sqrt(2.0 * math.Log(float64(len(nonzero)+1)) / (observations - 1))
			absoluteGain := math.Abs(selected.Correlation) - math.Abs(zero.Correlation)
			leads := math.Abs(selected.Correlation) > searchScale && absoluteGain > 0
			lagFraction := 0.0

			if leads {
				lagFraction = math.Abs(selected.LagIndex) / span
			}

			shapeIdx := int(selected.Index)
			shapeDefined := false
			prominence := 0.0
			curvature := 0.0

			if selected.Index > 0 && selected.Index < span*2 && shapeIdx > 0 && shapeIdx < len(candidates)-1 {
				lower := candidates[shapeIdx-1]
				upper := candidates[shapeIdx+1]

				if lower.Defined && upper.Defined {
					leftVal := math.Abs(lower.Y)
					centerVal := math.Abs(candidates[shapeIdx].Y)
					rightVal := math.Abs(upper.Y)
					diff := 2.0*centerVal - leftVal - rightVal
					seconds := spacing * 1e-9

					shapeDefined = true
					prominence = diff / 2.0
					curvature = diff / (seconds * seconds)
				}
			}

			leadLag.out = LeadLagReading{
				LagCandidate:    selected,
				Profile:         candidates,
				Defined:         true,
				Leads:           leads,
				ShapeDefined:    shapeDefined,
				Contemporaneous: zero.Correlation,
				SearchCount:     float64(len(nonzero)),
				Spacing:         spacing,
				Span:            span,
				Observations:    observations,
				SearchScale:     searchScale,
				AbsoluteGain:    absoluteGain,
				LagFraction:     lagFraction,
				Prominence:      prominence,
				Curvature:       curvature,
			}

			if !yield(unsafe.Pointer(&leadLag.out)) {
				return
			}
		}
	}
}

func medianSpacing(prices []temporal.Price) float64 {
	if len(prices) < 2 {
		return 0
	}

	spacings := make([]float64, len(prices)-1)

	for i := 1; i < len(prices); i++ {
		spacings[i-1] = float64(prices[i].At - prices[i-1].At)
	}

	slices.Sort(spacings)
	count := len(spacings)
	return (spacings[(count-1)/2] + spacings[count/2]) * 0.5
}
