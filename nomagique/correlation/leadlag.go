package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
minimumLagObservations is the smallest retained path that still yields a
correlation once a shift consumes the edges: two returns to correlate plus
the observation the shift costs. A structural floor of the estimator.
*/
const minimumLagObservations = 3

/*
twoSidedTails is the 2 in the Gaussian tail bound sqrt(2·ln(M)/N), from the
two tails of the null distribution.
*/
const twoSidedTails = 2

/*
LeadLag searches which temporal shift between two paths maximizes their
Hayashi-Yoshida correlation, and whether that peak is distinguishable from
the contemporaneous relationship.

The search grid is emergent: the shift step is the median of the two paths'
own sampling spacings, and the span is bounded by how much path is retained.
No lag range or resolution is configured.

Step returns the correlation at the peak shift. The peak's position,
significance and shape are exposed as accessors.

Degenerate behavior: an omitted path slot yields 0.
*/
type LeadLag struct {
	Left  *Path
	Right *Path

	hayashi  Hayashi
	shift    shiftHolder
	spacings []types.Number

	// Retained per-shift scan so the peak's support and its neighbours are
	// read back from the one pass rather than re-estimated.
	correlations []types.Number
	supports     []types.Number
	defined      []bool

	bestLag         int
	bestCorrelation types.Number
	contemporaneous types.Number
	searchCount     int
	observations    int
	maximumLag      int

	spacing                     types.Number
	significance                types.Number
	contemporaneousSignificance types.Number

	bestSupport  types.Number
	leftReturns  types.Number
	rightReturns types.Number

	leads bool
	ready bool
}

/*
shiftHolder is the Shift slot Hayashi reads. The scan writes the candidate
shift into it before each step, so Hayashi stays a pure composition target
rather than growing a scan-specific entry point.
*/
type shiftHolder struct {
	nanos types.Number
}

func (holder *shiftHolder) Step(types.Number) types.Number { return holder.nanos }

func (leadLag *LeadLag) Step(x types.Number) types.Number {
	leadLag.reset()

	if leadLag.Left == nil || leadLag.Right == nil {
		return 0
	}

	leadLag.hayashi.Left = leadLag.Left
	leadLag.hayashi.Right = leadLag.Right
	leadLag.hayashi.Shift = &leadLag.shift

	leadLag.observations = min(leadLag.Left.Len(), leadLag.Right.Len())

	if leadLag.observations < minimumLagObservations {
		return 0
	}

	// The finer of the two emergent resolutions: a step coarser than either
	// path's own sampling would stride straight over the true peak.
	leadLag.spacings = leadLag.Left.Spacings(leadLag.spacings)
	leftSpacing := statistic.MedianReduction(leadLag.spacings)

	leadLag.spacings = leadLag.Right.Spacings(leadLag.spacings)
	rightSpacing := statistic.MedianReduction(leadLag.spacings)

	if leftSpacing <= 0 || rightSpacing <= 0 {
		return 0
	}

	leadLag.spacing = types.Number(math.Min(float64(leftSpacing), float64(rightSpacing)))
	leadLag.maximumLag = leadLag.observations - minimumLagObservations + 1

	leadLag.scan()

	if leadLag.searchCount == 0 {
		return 0
	}

	leadLag.significance = leadLag.bound(float64(leadLag.searchCount + 1))
	leadLag.contemporaneousSignificance = leadLag.bound(2)

	magnitude := types.Number(math.Abs(float64(leadLag.bestCorrelation)))

	// A lead-lag relationship is claimed only when the peak clears its own
	// search-adjusted threshold AND beats the contemporaneous relationship.
	leadLag.leads = magnitude > leadLag.significance &&
		magnitude > types.Number(math.Abs(float64(leadLag.contemporaneous)))

	leadLag.ready = true

	return leadLag.bestCorrelation
}

/*
scan steps Hayashi across every admissible shift, retaining each result.
*/
func (leadLag *LeadLag) scan() {
	leadLag.size(2*leadLag.maximumLag + 1)

	best := types.Number(0)

	for lag := -leadLag.maximumLag; lag <= leadLag.maximumLag; lag++ {
		leadLag.shift.nanos = types.Number(lag) * leadLag.spacing

		correlation := leadLag.hayashi.Step(0)
		defined := leadLag.hayashi.Ready()

		index := lag + leadLag.maximumLag
		leadLag.correlations[index] = correlation
		leadLag.supports[index] = leadLag.hayashi.Support()
		leadLag.defined[index] = defined

		if !defined {
			continue
		}

		if lag == 0 {
			leadLag.contemporaneous = correlation
			leadLag.leftReturns = leadLag.hayashi.LeftReturns()
			leadLag.rightReturns = leadLag.hayashi.RightReturns()

			continue
		}

		leadLag.searchCount++
		magnitude := types.Number(math.Abs(float64(correlation)))

		if magnitude <= best {
			continue
		}

		best = magnitude
		leadLag.bestLag = lag
		leadLag.bestCorrelation = correlation
	}

	if leadLag.searchCount > 0 {
		leadLag.bestSupport = leadLag.supports[leadLag.bestLag+leadLag.maximumLag]
	}
}

/*
bound returns the Bonferroni threshold sqrt(2·ln(M)/N) for M candidates over
N returns: the bar a peak must clear grows with the size of the search that
found it.
*/
func (leadLag *LeadLag) bound(candidates float64) types.Number {
	returns := float64(leadLag.observations - 1)

	if returns <= 0 {
		return 0
	}

	return types.Number(math.Sqrt(twoSidedTails * math.Log(candidates) / returns))
}

func (leadLag *LeadLag) size(count int) {
	if cap(leadLag.correlations) >= count {
		leadLag.correlations = leadLag.correlations[:count]
		leadLag.supports = leadLag.supports[:count]
		leadLag.defined = leadLag.defined[:count]
	} else {
		leadLag.correlations = make([]types.Number, count)
		leadLag.supports = make([]types.Number, count)
		leadLag.defined = make([]bool, count)
	}

	for index := range leadLag.correlations {
		leadLag.correlations[index] = 0
		leadLag.supports[index] = 0
		leadLag.defined[index] = false
	}
}

func (leadLag *LeadLag) reset() {
	leadLag.bestLag = 0
	leadLag.bestCorrelation = 0
	leadLag.contemporaneous = 0
	leadLag.searchCount = 0
	leadLag.observations = 0
	leadLag.maximumLag = 0
	leadLag.spacing = 0
	leadLag.significance = 0
	leadLag.contemporaneousSignificance = 0
	leadLag.bestSupport = 0
	leadLag.leftReturns = 0
	leadLag.rightReturns = 0
	leadLag.leads = false
	leadLag.ready = false
}

// Ready reports whether the last step produced a defined estimate.
func (leadLag *LeadLag) Ready() bool { return leadLag.ready }

// LagBars returns the peak shift in units of the emergent search spacing.
func (leadLag *LeadLag) LagBars() types.Number { return types.Number(leadLag.bestLag) }

// LagNanos returns the peak shift in nanoseconds.
func (leadLag *LeadLag) LagNanos() types.Number {
	return types.Number(leadLag.bestLag) * leadLag.spacing
}

// LagCorrelation returns the correlation at the peak shift.
func (leadLag *LeadLag) LagCorrelation() types.Number { return leadLag.bestCorrelation }

// Contemporaneous returns the correlation at zero shift.
func (leadLag *LeadLag) Contemporaneous() types.Number { return leadLag.contemporaneous }

/*
Leads reports whether the peak is a genuine lead-lag relationship: it clears
the search-adjusted bound and exceeds the contemporaneous correlation.
*/
func (leadLag *LeadLag) Leads() bool { return leadLag.leads }

/*
AbsoluteGain returns how much correlation the best shift buys over the
contemporaneous relationship: |peak| - |contemporaneous|.
*/
func (leadLag *LeadLag) AbsoluteGain() types.Number {
	return types.Number(math.Abs(float64(leadLag.bestCorrelation)) -
		math.Abs(float64(leadLag.contemporaneous)))
}

/*
LagFraction returns how far into the searched span the peak sits, in [0, 1].
It is zero when no lead-lag relationship was established, so an insignificant
peak never reports a position.
*/
func (leadLag *LeadLag) LagFraction() types.Number {
	if !leadLag.leads || leadLag.maximumLag == 0 {
		return 0
	}

	return types.Number(math.Abs(float64(leadLag.bestLag)) / float64(leadLag.maximumLag))
}

// Significance returns the Bonferroni threshold the peak had to clear.
func (leadLag *LeadLag) Significance() types.Number { return leadLag.significance }

// ContemporaneousSignificance returns the two-sided bound at zero shift.
func (leadLag *LeadLag) ContemporaneousSignificance() types.Number {
	return leadLag.contemporaneousSignificance
}

// SearchCount returns how many non-zero shifts yielded a defined estimate.
func (leadLag *LeadLag) SearchCount() types.Number {
	return types.Number(leadLag.searchCount)
}

// Observations returns the shorter of the two paths' retained lengths.
func (leadLag *LeadLag) Observations() types.Number {
	return types.Number(leadLag.observations)
}

// Support returns the overlapping return pairs behind the peak estimate.
func (leadLag *LeadLag) Support() types.Number { return leadLag.bestSupport }

// SearchResolution returns the emergent shift step in nanoseconds.
func (leadLag *LeadLag) SearchResolution() types.Number { return leadLag.spacing }

// SearchResolutionSeconds returns the emergent shift step in seconds.
func (leadLag *LeadLag) SearchResolutionSeconds() types.Number {
	return leadLag.spacing / NanosPerSecond
}

// SearchSpan returns the total scanned shift range in nanoseconds.
func (leadLag *LeadLag) SearchSpan() types.Number {
	return leadLag.spacing * types.Number(leadLag.maximumLag)
}

// SearchSpanSeconds returns the total scanned shift range in seconds.
func (leadLag *LeadLag) SearchSpanSeconds() types.Number {
	return leadLag.SearchSpan() / NanosPerSecond
}

// LagSeconds returns the peak shift in seconds.
func (leadLag *LeadLag) LagSeconds() types.Number {
	return leadLag.LagNanos() / NanosPerSecond
}

// LeftReturns returns the valid return intervals on the Left path.
func (leadLag *LeadLag) LeftReturns() types.Number { return leadLag.leftReturns }

// RightReturns returns the valid return intervals on the Right path.
func (leadLag *LeadLag) RightReturns() types.Number { return leadLag.rightReturns }

/*
PeakProminence returns how far the peak stands above its immediate
neighbours — how isolated the maximum is, as opposed to how high. It reports
false when either neighbour falls outside the scan or was undefined.
*/
func (leadLag *LeadLag) PeakProminence() (types.Number, bool) {
	low, high, ok := leadLag.neighbours()

	if !ok {
		return 0, false
	}

	peak := math.Abs(float64(leadLag.bestCorrelation))

	return types.Number(peak - (math.Abs(float64(low))+math.Abs(float64(high)))/2), true
}

/*
PeakCurvature returns the second difference across the peak normalized by the
squared search resolution in seconds: how sharply the relationship falls away
from its best shift.
*/
func (leadLag *LeadLag) PeakCurvature() (types.Number, bool) {
	low, high, ok := leadLag.neighbours()

	if !ok || leadLag.spacing <= 0 {
		return 0, false
	}

	peak := math.Abs(float64(leadLag.bestCorrelation))
	seconds := float64(leadLag.spacing) / NanosPerSecond

	curvature := (2*peak - math.Abs(float64(low)) - math.Abs(float64(high))) /
		(seconds * seconds)

	return types.Number(curvature), true
}

func (leadLag *LeadLag) neighbours() (types.Number, types.Number, bool) {
	if !leadLag.ready {
		return 0, 0, false
	}

	index := leadLag.bestLag + leadLag.maximumLag

	if index-1 < 0 || index+1 >= len(leadLag.correlations) {
		return 0, 0, false
	}

	if !leadLag.defined[index-1] || !leadLag.defined[index+1] {
		return 0, 0, false
	}

	return leadLag.correlations[index-1], leadLag.correlations[index+1], true
}

var _ types.Node = (*LeadLag)(nil)
