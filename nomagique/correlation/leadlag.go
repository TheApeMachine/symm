package correlation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LeadLagReading is one lead/lag search. Empty searches are explicit undefined
records. x is seconds, spacing is nanoseconds.
*/
type LeadLagReading struct {
	equation.LagCandidate
	Profile         []equation.LagCandidate
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
Resolution is the finer path's median spacing; span is min(path counts)-2.
*/
type LeadLag struct {
	core.Base[equation.LagProfileInput, LeadLagReading]
	estimator equation.LagEstimator
	peak      *equation.Peak
	scale     *equation.SearchScale
	shape     *LagShape
}

func NewLeadLag(estimator equation.LagEstimator) *LeadLag {
	return &LeadLag{
		estimator: estimator,
		peak:      equation.NewPeak(),
		scale:     equation.NewSearchScale(),
		shape:     NewLagShape(),
	}
}

func (op *LeadLag) Next(
	in iter.Seq[core.Primitive[equation.LagProfileInput, equation.LagProfileInput]],
) iter.Seq[core.Primitive[LeadLagReading, LeadLagReading]] {
	return func(yield func(core.Primitive[LeadLagReading, LeadLagReading]) bool) {
		for arriving := range in {
			reading, err := op.Search(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *LeadLag) Search(input equation.LagProfileInput) (LeadLagReading, error) {
	undefined := LeadLagReading{Profile: []equation.LagCandidate{}}
	observations := math.Min(float64(len(input.Left)), float64(len(input.Right)))

	if !(observations > 2) {
		return undefined, nil
	}

	leftSpacing, err := op.medianSpacing(input.Left)

	if err != nil {
		return undefined, nil
	}

	rightSpacing, err := op.medianSpacing(input.Right)

	if err != nil {
		return undefined, nil
	}

	spacing := math.Min(leftSpacing, rightSpacing)

	if !(spacing > 0) {
		return undefined, nil
	}

	span := observations - 2
	profile := equation.NewLagProfile(NewDependence(op.estimator), int64(spacing), span)
	candidates := make([]equation.LagCandidate, 0)
	nonzero := make([]equation.LagCandidate, 0)

	for candidate := range profile.Next(transport.Values(input)) {
		current := candidate.Read()
		candidates = append(candidates, current)

		if current.Defined() && current.X != 0 {
			nonzero = append(nonzero, current)
		}
	}

	if err := profile.Error(); err != nil {
		return LeadLagReading{}, err
	}

	if len(nonzero) == 0 {
		return undefined, nil
	}

	points := make([]equation.Point, len(nonzero))

	for index, candidate := range nonzero {
		points[index] = equation.Point{X: candidate.X, Y: candidate.Y}
	}

	peak, err := transport.Evaluate(op.peak, transport.Values(points...))

	if err != nil {
		return LeadLagReading{}, err
	}

	selected := nonzero[peak.Index]
	zero := candidates[int(span)]
	searchScale, err := transport.Evaluate(op.scale, transport.Values(equation.SearchScaleInput{
		Candidates:   float64(len(nonzero) + 1),
		Observations: observations - 1,
	}))

	if err != nil {
		return LeadLagReading{}, err
	}

	absoluteGain := math.Abs(selected.Correlation) - math.Abs(zero.Correlation)
	leads := math.Abs(selected.Correlation) > searchScale && absoluteGain > 0
	lagFraction := 0.0

	if leads {
		lagFraction = math.Abs(selected.LagIndex) / span
	}

	shape, err := op.shape.Evaluate(LagShapeInput{
		Profile: candidates,
		Index:   selected.Index,
		Span:    span,
		Spacing: spacing,
	})

	if err != nil {
		return LeadLagReading{}, err
	}

	return LeadLagReading{
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
	}, nil
}

func (op *LeadLag) medianSpacing(prices []equation.Price) (float64, error) {
	stamps := make([]equation.Stamp, len(prices))

	for index, price := range prices {
		stamps[index] = equation.Stamp{At: price.At}
	}

	return transport.Evaluate(equation.NewMedian[float64](), equation.NewSpacings().Next(transport.Values(stamps...)))
}
