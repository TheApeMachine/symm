package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	spacing     core.Primitive
	searchScale core.Primitive
	shape       core.Primitive
}

/*
NewLeadLag creates a new LeadLag primitive over the supplied estimator.
*/
func NewLeadLag(estimator core.Primitive) *LeadLag {
	return &LeadLag{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
		spacing:        nomagique.NewNumber(temporal.NewSpacings(), statistic.NewMedian()),
		searchScale:    NewSearchScale(),
		shape:          NewLagShape(),
	}
}

func (leadLag *LeadLag) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			undefined := LeadLagReading{Profile: []LagCandidate{}}
			observations := math.Min(float64(len(input.Left)), float64(len(input.Right)))
			minObservations := core.Unit + core.Unit

			if observations <= minObservations {
				if !yield(unsafe.Pointer(&undefined)) {
					return
				}

				continue
			}

			leftSpacing := 0.0

			if float64(len(input.Left)) >= minObservations {
				ats := make([]int64, len(input.Left))

				for index, price := range input.Left {
					ats[index] = price.At
				}

				for out := range leadLag.spacing.Next(sequence.NewValues(ats...).Next(nil)) {
					leftSpacing = *(*float64)(out)
				}
			}

			rightSpacing := 0.0

			if float64(len(input.Right)) >= minObservations {
				ats := make([]int64, len(input.Right))

				for index, price := range input.Right {
					ats[index] = price.At
				}

				for out := range leadLag.spacing.Next(sequence.NewValues(ats...).Next(nil)) {
					rightSpacing = *(*float64)(out)
				}
			}

			spacing := math.Min(leftSpacing, rightSpacing)

			if spacing <= 0 {
				if !yield(unsafe.Pointer(&undefined)) {
					return
				}

				continue
			}

			span := observations - minObservations
			profileOp := NewLagProfile(leadLag.estimator, int64(spacing), span)
			var candidates []LagCandidate
			var nonzero []LagCandidate

			for out := range profileOp.Next(sequence.NewOne(arriving).Next(nil)) {
				candidate := *(*LagCandidate)(out)
				candidates = append(candidates, candidate)

				if candidate.Defined && candidate.X != 0 {
					nonzero = append(nonzero, candidate)
				}
			}

			if err := profileOp.Error(); err != nil {
				leadLag.Error(err)
				return
			}

			if len(nonzero) == 0 {
				if !yield(unsafe.Pointer(&undefined)) {
					return
				}

				continue
			}

			bestIdx := 0
			maxMag := math.Abs(nonzero[0].Y)

			for index := 1; index < len(nonzero); index++ {
				mag := math.Abs(nonzero[index].Y)

				if mag > maxMag {
					maxMag = mag
					bestIdx = index
				}
			}

			selected := nonzero[bestIdx]
			zero := candidates[int(span)]

			scaleInput := SearchScaleInput{
				Candidates:   float64(len(nonzero)) + core.Unit,
				Observations: observations - core.Unit,
			}
			var searchScale float64

			for out := range leadLag.searchScale.Next(sequence.NewOne(unsafe.Pointer(&scaleInput)).Next(nil)) {
				searchScale = *(*float64)(out)
			}

			shapeInput := LagShapeInput{
				Profile: candidates,
				Index:   selected.Index,
				Span:    span,
				Spacing: spacing,
			}
			var shape LagShapeResult

			for out := range leadLag.shape.Next(sequence.NewOne(unsafe.Pointer(&shapeInput)).Next(nil)) {
				shape = *(*LagShapeResult)(out)
			}

			absoluteGain := math.Abs(selected.Correlation) - math.Abs(zero.Correlation)
			leads := math.Abs(selected.Correlation) > searchScale && absoluteGain > 0
			lagFraction := 0.0

			if span > 0 {
				lagFraction = math.Abs(selected.LagIndex) / span
			}

			reading := LeadLagReading{
				LagCandidate:    selected,
				Profile:         candidates,
				Defined:         true,
				Leads:           leads,
				ShapeDefined:    shape.ShapeDefined,
				Contemporaneous: zero.Correlation,
				SearchCount:     float64(len(nonzero)),
				Spacing:         spacing,
				Span:            span,
				Observations:    observations,
				SearchScale:     searchScale,
				AbsoluteGain:    absoluteGain,
				LagFraction:     lagFraction,
				Prominence:      shape.Prominence,
				Curvature:       shape.Curvature,
			}

			if !yield(unsafe.Pointer(&reading)) {
				return
			}
		}
	}
}
