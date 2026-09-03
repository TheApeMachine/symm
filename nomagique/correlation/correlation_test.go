package correlation

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func newPath(nanos []int64, values []float64) *Path {
	path := &Path{}

	for index := range nanos {
		path.Observe(nanos[index], types.Number(values[index]))
	}

	return path
}

/*
TestHayashiProportionalPaths reproduces the v1 acceptance case: two paths on
offset clocks moving in exact proportion correlate at 1 across every
overlapping return interval.
*/
func TestHayashiProportionalPaths(t *testing.T) {
	left := newPath(
		[]int64{0, 1_000_000_000, 2_000_000_000, 3_000_000_000},
		[]float64{100, 110, 121, 133.1},
	)
	right := newPath(
		[]int64{1, 1_000_000_001, 2_000_000_001, 3_000_000_001},
		[]float64{50, 55, 60.5, 66.55},
	)

	hayashi := &Hayashi{Left: left, Right: right}

	if got := hayashi.Step(0); math.Abs(float64(got)-1) > 1e-9 {
		t.Fatalf("correlation = %v, want 1", got)
	}

	if !hayashi.Ready() {
		t.Fatal("proportional paths must produce a defined correlation")
	}

	if hayashi.Support() != 5 {
		t.Fatalf("support = %v, want 5", hayashi.Support())
	}
}

func TestHayashiAntiCorrelation(t *testing.T) {
	nanos := []int64{0, 1_000_000_000, 2_000_000_000, 3_000_000_000}
	left := newPath(nanos, []float64{100, 110, 121, 133.1})
	right := newPath(nanos, []float64{100, 90.909, 82.644, 75.131})

	if got := (&Hayashi{Left: left, Right: right}).Step(0); got > -0.99 {
		t.Fatalf("correlation = %v, want approximately -1", got)
	}
}

/*
TestHayashiDegenerateSlots proves the zero-value rule: an omitted path slot
contributes no dependence rather than panicking.
*/
func TestHayashiDegenerateSlots(t *testing.T) {
	path := newPath([]int64{0, 1_000_000_000}, []float64{100, 110})

	if got := (&Hayashi{}).Step(0); got != 0 {
		t.Fatalf("empty Hayashi = %v, want 0", got)
	}

	if got := (&Hayashi{Left: path}).Step(0); got != 0 {
		t.Fatalf("half-configured Hayashi = %v, want 0", got)
	}
}

func TestHayashiRejectsVarianceFreePath(t *testing.T) {
	nanos := []int64{0, 1_000_000_000, 2_000_000_000}
	flat := newPath(nanos, []float64{100, 100, 100})
	moving := newPath(nanos, []float64{100, 110, 121})

	hayashi := &Hayashi{Left: flat, Right: moving}

	if got := hayashi.Step(0); got != 0 {
		t.Fatalf("correlation = %v, want 0", got)
	}

	if hayashi.Ready() {
		t.Fatal("a variance-free path must not produce a defined correlation")
	}
}

/*
TestHayashiShiftSlotComposes proves Shift is a real node slot: driving it
with a Constant reproduces a known lag alignment.
*/
func TestHayashiShiftSlotComposes(t *testing.T) {
	const spacing = 1_000_000_000

	nanos := make([]int64, 12)
	leftValues := make([]float64, 12)
	rightValues := make([]float64, 12)

	walk := make([]float64, 14)
	price := 100.0

	for index := range walk {
		price *= 1 + 0.02*math.Sin(float64(index))
		walk[index] = price
	}

	for index := range nanos {
		nanos[index] = int64(index) * spacing
		leftValues[index] = walk[index+2]
		rightValues[index] = walk[index]
	}

	left := newPath(nanos, leftValues)
	right := newPath(nanos, rightValues)

	unshifted := (&Hayashi{Left: left, Right: right}).Step(0)

	shifted := (&Hayashi{
		Left:  left,
		Right: right,
		Shift: calculus.Constant{Value: -2 * spacing},
	}).Step(0)

	if math.Abs(float64(shifted)) <= math.Abs(float64(unshifted)) {
		t.Fatalf("shift-aligned correlation %v must exceed unshifted %v",
			shifted, unshifted)
	}
}

func TestHayashiSharedTimeAndDensity(t *testing.T) {
	left := newPath(
		[]int64{0, 1_000_000_000, 2_000_000_000},
		[]float64{100, 110, 121},
	)
	right := newPath(
		[]int64{1_000_000_000, 2_000_000_000, 3_000_000_000},
		[]float64{50, 55, 60.5},
	)

	hayashi := &Hayashi{Left: left, Right: right}
	hayashi.Step(0)

	if math.Abs(float64(hayashi.SharedTime())-1) > 1e-9 {
		t.Fatalf("shared time = %v, want 1", hayashi.SharedTime())
	}

	expected := hayashi.Support() / hayashi.SharedTime()

	if hayashi.OverlapDensity() != expected {
		t.Fatalf("overlap density = %v, want %v", hayashi.OverlapDensity(), expected)
	}
}

func TestHayashiDisjointPathsShareNoTime(t *testing.T) {
	left := newPath([]int64{0, 1_000_000_000}, []float64{100, 110})
	right := newPath([]int64{5_000_000_000, 6_000_000_000}, []float64{50, 55})

	hayashi := &Hayashi{Left: left, Right: right}
	hayashi.Step(0)

	if hayashi.SharedTime() != 0 {
		t.Fatalf("shared time = %v, want 0", hayashi.SharedTime())
	}

	if hayashi.OverlapDensity() != 0 {
		t.Fatalf("overlap density = %v, want 0", hayashi.OverlapDensity())
	}
}

/*
TestPathRefusesRegressingTime proves an out-of-order observation is refused
rather than silently corrupting every derived return interval.
*/
func TestPathRefusesRegressingTime(t *testing.T) {
	path := &Path{}

	if !path.Observe(2_000_000_000, 100) {
		t.Fatal("the first observation must be accepted")
	}

	if path.Observe(1_000_000_000, 101) {
		t.Fatal("a regressing observation must be refused")
	}

	if path.Len() != 1 {
		t.Fatalf("path length = %d, want 1", path.Len())
	}

	// A repeated instant restates the same observation in place.
	if !path.Observe(2_000_000_000, 105) {
		t.Fatal("a repeated instant must be accepted")
	}

	if path.Len() != 1 {
		t.Fatalf("path length = %d, want 1 after restatement", path.Len())
	}
}

/*
TestPathIsAlgebraicSink proves the Law of Sinks: a Path with no Reduce slot
returns 0, so it records inside a Split without disturbing the sum.
*/
func TestPathIsAlgebraicSink(t *testing.T) {
	path := &Path{}
	path.Observe(0, 100)

	if got := path.Step(100); got != 0 {
		t.Fatalf("sink Path = %v, want 0", got)
	}

	split := &types.Split{
		A: calculus.Constant{Value: 42},
		B: path,
	}

	if got := split.Step(100); got != 42 {
		t.Fatalf("Split with a sink = %v, want 42 uncorrupted", got)
	}
}

/*
TestPathGrowsBeyondV1Clamp proves the fixed-width clamp is gone: a path far
longer than v1's MaxPathSamples retains more than 128 observations under a
stationary stream.
*/
func TestPathGrowsBeyondV1Clamp(t *testing.T) {
	path := &Path{}

	for index := range 5000 {
		path.Observe(int64(index)*1_000_000, types.Number(100+index%3))
	}

	if path.Len() <= 128 {
		t.Fatalf("retained %d observations, want more than the v1 clamp of 128",
			path.Len())
	}
}

/*
TestPathHorizonContractsOnDrift proves the Horizon slot is real: an ADWIN
horizon sheds observations when the stream drifts, while the same stream with
no Horizon retains everything.
*/
func TestPathHorizonContractsOnDrift(t *testing.T) {
	bounded := &Path{Horizon: &adaptive.Window{Type: adaptive.ADWIN}}
	unbounded := &Path{}

	for index := range 200 {
		value := types.Number(100 + index%3)
		nanos := int64(index) * 1_000_000

		bounded.Observe(nanos, value)
		unbounded.Observe(nanos, value)
	}

	if unbounded.Len() != 200 {
		t.Fatalf("unbounded path retained %d, want all 200", unbounded.Len())
	}

	if bounded.Len() >= unbounded.Len() {
		t.Fatalf("bounded path retained %d, want fewer than the unbounded %d",
			bounded.Len(), unbounded.Len())
	}
}

/*
TestLeadLagRecoversKnownShift proves the search finds a real lead: the right
path is the left path delayed by two sampling intervals.
*/
func TestLeadLagRecoversKnownShift(t *testing.T) {
	const samples = 40
	const spacing = 1_000_000_000
	const delay = 2

	nanos := make([]int64, samples)
	leftValues := make([]float64, samples)
	rightValues := make([]float64, samples)

	walk := make([]float64, samples+delay)
	price := 100.0

	for index := range walk {
		price *= 1 + 0.01*math.Sin(float64(index))
		walk[index] = price
	}

	for index := range nanos {
		nanos[index] = int64(index) * spacing
		leftValues[index] = walk[index+delay]
		rightValues[index] = walk[index]
	}

	leadLag := &LeadLag{
		Left:  newPath(nanos, leftValues),
		Right: newPath(nanos, rightValues),
	}

	correlation := leadLag.Step(0)

	if !leadLag.Ready() {
		t.Fatal("a delayed pair must produce a defined lead-lag estimate")
	}

	if leadLag.LagBars() == 0 {
		t.Fatal("a genuinely delayed pair must not peak at zero lag")
	}

	if math.Abs(float64(correlation)) <=
		math.Abs(float64(leadLag.Contemporaneous())) {
		t.Fatalf("peak |%v| must exceed contemporaneous |%v|",
			correlation, leadLag.Contemporaneous())
	}

	if leadLag.SearchResolution() != spacing {
		t.Fatalf("search resolution = %v, want the emergent spacing %d",
			leadLag.SearchResolution(), spacing)
	}

	if leadLag.AbsoluteGain() <= 0 {
		t.Fatalf("absolute gain = %v, want positive", leadLag.AbsoluteGain())
	}
}

func TestLeadLagDegenerateSlots(t *testing.T) {
	if got := (&LeadLag{}).Step(0); got != 0 {
		t.Fatalf("empty LeadLag = %v, want 0", got)
	}
}

func TestLeadLagRejectsShortPaths(t *testing.T) {
	short := newPath([]int64{0, 1_000_000_000}, []float64{100, 110})

	leadLag := &LeadLag{Left: short, Right: short}

	if got := leadLag.Step(0); got != 0 {
		t.Fatalf("short-path estimate = %v, want 0", got)
	}

	if leadLag.Ready() {
		t.Fatal("a two-sample path must not produce a lead-lag estimate")
	}

	if leadLag.LagFraction() != 0 {
		t.Fatalf("lag fraction = %v, want 0", leadLag.LagFraction())
	}
}

/*
TestFisherSignificance proves the transform, its standard error, and the
Bonferroni correction, all driven through node slots.
*/
func TestFisherSignificance(t *testing.T) {
	fisher := &Fisher{
		Support:     calculus.Constant{Value: 40},
		SearchCount: calculus.Constant{Value: 1},
	}

	pValue := fisher.Step(0.8)

	if !fisher.Ready() {
		t.Fatal("a well-supported correlation must produce defined statistics")
	}

	if pValue <= 0 || pValue > 1 {
		t.Fatalf("p-value = %v, want within (0, 1]", pValue)
	}

	if pValue > 0.01 {
		t.Fatalf("p-value = %v, want a strongly significant result", pValue)
	}

	expectedError := 1 / math.Sqrt(40-3)

	if math.Abs(float64(fisher.StandardError())-expectedError) > 1e-12 {
		t.Fatalf("standard error = %v, want %v", fisher.StandardError(), expectedError)
	}

	searched := &Fisher{
		Support:     calculus.Constant{Value: 40},
		SearchCount: calculus.Constant{Value: 100},
	}
	searched.Step(0.8)

	adjusted, ok := searched.SearchAdjustedPValue()

	if !ok {
		t.Fatal("a searched estimate must carry an adjusted p-value")
	}

	if adjusted <= searched.PValue() {
		t.Fatalf("adjusted p %v must exceed raw p %v", adjusted, searched.PValue())
	}

	if adjusted > 1 {
		t.Fatalf("adjusted p = %v, want capped at 1", adjusted)
	}
}

func TestFisherHonestSupport(t *testing.T) {
	thin := &Fisher{Support: calculus.Constant{Value: 3}}

	if got := thin.Step(0.9); got != 0 {
		t.Fatalf("thin-support p = %v, want 0", got)
	}

	if thin.Ready() {
		t.Fatal("support below the Fisher floor must not produce statistics")
	}

	saturated := &Fisher{Support: calculus.Constant{Value: 40}}

	if got := saturated.Step(1); got != 0 {
		t.Fatalf("saturated-correlation p = %v, want 0", got)
	}

	// An omitted Support slot cannot establish significance.
	if got := (&Fisher{}).Step(0.8); got != 0 {
		t.Fatalf("unsupported Fisher = %v, want 0", got)
	}
}

/*
supportHolder drives Cohort's Support slot per observed peer.
*/
type supportHolder struct{ value types.Number }

func (holder *supportHolder) Step(types.Number) types.Number { return holder.value }

func TestCohortWeightsBySupport(t *testing.T) {
	support := &supportHolder{}
	cohort := &Cohort{Support: support}

	// A strong correlation on thin support, then a weak one on thick support.
	support.value = 2
	cohort.Step(0.9)

	support.value = 100
	cohort.Step(0.1)

	if !cohort.Ready() {
		t.Fatal("observed peers must define the aggregate")
	}

	if cohort.Peers() != 2 {
		t.Fatalf("peers = %v, want 2", cohort.Peers())
	}

	// The thickly-supported weak peer must dominate the naive mean of 0.5.
	if cohort.SignedCorrelation() > 0.2 {
		t.Fatalf("signed correlation = %v, want dominated by the supported peer",
			cohort.SignedCorrelation())
	}

	if cohort.EffectivePeers() >= 2 {
		t.Fatalf("effective peers = %v, want below the nominal 2",
			cohort.EffectivePeers())
	}
}

func TestCohortSignedVersusAbsolute(t *testing.T) {
	cohort := &Cohort{Support: calculus.Constant{Value: 10}}

	cohort.Step(0.8)
	cohort.Step(-0.8)

	if math.Abs(float64(cohort.SignedCorrelation())) > 1e-12 {
		t.Fatalf("signed correlation = %v, want 0", cohort.SignedCorrelation())
	}

	if math.Abs(float64(cohort.AbsoluteCorrelation())-0.8) > 1e-12 {
		t.Fatalf("absolute correlation = %v, want 0.8", cohort.AbsoluteCorrelation())
	}

	if cohort.Dispersion() <= 0 {
		t.Fatalf("dispersion = %v, want positive for a split cohort",
			cohort.Dispersion())
	}
}

func TestCohortIgnoresThinSupportAndResets(t *testing.T) {
	cohort := &Cohort{Support: calculus.Constant{Value: 1}}

	cohort.Step(0.99)

	if cohort.Ready() {
		t.Fatal("a single overlapping pair must not contribute")
	}

	// An omitted Support slot folds nothing.
	if got := (&Cohort{}).Step(0.9); got != 0 {
		t.Fatalf("unsupported Cohort = %v, want 0", got)
	}

	settled := &Cohort{Support: calculus.Constant{Value: 10}}
	settled.Step(0.5)

	if !settled.Ready() {
		t.Fatal("a supported peer must define the aggregate")
	}

	settled.Reset()

	if settled.Ready() || settled.SignedCorrelation() != 0 {
		t.Fatal("Reset must clear the accumulator")
	}
}

/*
TestCorrelationComposesAsPipeline proves the whole point: a correlation
estimate and its significance compose as one nomagique.Number Chain, with no
caller arithmetic between the stages.
*/
func TestCorrelationComposesAsPipeline(t *testing.T) {
	// Correlated but not perfectly: a saturated |r| = 1 leaves the Fisher
	// transform undefined, which is the correct refusal rather than a bug.
	nanos := make([]int64, 30)
	leftValues := make([]float64, 30)
	rightValues := make([]float64, 30)

	leftPrice, rightPrice := 100.0, 50.0

	for index := range nanos {
		shock := 0.01 * math.Sin(float64(index))
		leftPrice *= 1 + shock
		rightPrice *= 1 + shock*0.8 + 0.002*math.Cos(float64(index)*3)

		nanos[index] = int64(index) * 1_000_000_000
		leftValues[index] = leftPrice
		rightValues[index] = rightPrice
	}

	left := newPath(nanos, leftValues)
	right := newPath(nanos, rightValues)

	hayashi := &Hayashi{Left: left, Right: right}

	pipeline := &types.Chain{
		A: hayashi,
		B: &Fisher{Support: hayashi.SupportSlot()},
	}

	pValue := pipeline.Step(0)

	if pValue <= 0 || pValue > 1 {
		t.Fatalf("composed p-value = %v, want within (0, 1]", pValue)
	}
}

var (
	_ types.Node = (*Hayashi)(nil)
	_ types.Node = (*LeadLag)(nil)
	_ types.Node = (*Fisher)(nil)
	_ types.Node = (*Cohort)(nil)
	_ types.Node = (*Path)(nil)
)
