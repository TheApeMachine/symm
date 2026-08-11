package leadlag

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
)

const sampleFloor = 2

/*
priceSample is one observed price point on a symbol path.
*/
type priceSample struct {
	at    time.Time
	value float64
}

/*
Section tracks anchor-relative price paths for lead-lag scoring.

ponytail: Section is an intentional in-memory correlation index, not a tree-
backed history store. Lead-lag is a cross-sectional, anchor-relative computation
that needs every follower's aligned return path against the live leader on the
same tick; reconstructing N synchronized paths from measurement artifacts each
cycle would be an O(N·depth) tree replay per row. The ceiling: history is
process-local (not shared across replicas) and rebuilt cold on restart. Upgrade
path: persist per-symbol return paths as measurement replay fields and seek them
by `measurement/{symbol}/leadlag/` when cross-replica state is required.
*/
type Section struct {
	universe     sync.Map
	anchorSymbol string
	moveBaseline *algorithm.MoveBaseline
	lastMove     algorithm.MoveBaselineOutput
}

/*
symbolState retains aligned prices and returns for one market so lead-lag
calculations use coherent samples.
*/
type symbolState struct {
	last          float64
	lastSampleAt  time.Time
	prices        []priceSample
	observedCount int
}

/*
LagFeatures holds derived lag inputs for classification.
*/
type LagFeatures struct {
	IsAnchor     bool
	Price        float64
	MoveReady    bool
	MoveMoved    bool
	StallMargin  float64
	LagOK        bool
	LagBars      int
	LagCorr      float64
	ContempOK    bool
	ContempCorr  float64
	SampleCount  int
	ObservedFrom time.Time
	ObservedAt   time.Time
}

/*
NewSection creates a Section with no anchor. The anchor is derived live from the
cross-section leader on every Measure cycle (Section.SetAnchor), so there is no
config anchor and no fixed major to seed.
*/
func NewSection() *Section {
	return &Section{
		moveBaseline: newMoveBaseline(),
	}
}

/*
AnchorSymbol returns the cohort anchor so callers can expose the active
reference market.
*/
func (section *Section) AnchorSymbol() string {
	return section.anchorSymbol
}

/*
SetAnchor switches the lead-lag anchor to the current cross-section leader.
When leadership rotates to a new symbol the buffered anchor-move history is
anchor-specific, so it is reset — the new leader's moves seed a fresh baseline.
*/
func (section *Section) SetAnchor(symbol string) {
	if symbol == "" || symbol == section.anchorSymbol {
		return
	}

	if section.anchorSymbol != "" {
		section.moveBaseline = newMoveBaseline()
		section.lastMove = algorithm.MoveBaselineOutput{}
	}

	section.anchorSymbol = symbol
}

/*
ClearAnchor drops the live leader so leaderless cuts emit provisional evidence.
*/
func (section *Section) ClearAnchor() {
	if section.anchorSymbol == "" {
		return
	}

	section.moveBaseline = newMoveBaseline()
	section.lastMove = algorithm.MoveBaselineOutput{}
	section.anchorSymbol = ""
}

/*
PriceSampleCount returns how many spaced price samples are buffered for symbol.
*/
func (section *Section) PriceSampleCount(symbol string) int {
	state := section.ensure(symbol)

	if state == nil {
		return 0
	}

	return len(state.prices)
}

/*
ensure returns existing symbol state or creates owned state so observations
share one timeline.
*/
func (section *Section) ensure(symbol string) *symbolState {
	raw, _ := section.universe.LoadOrStore(symbol, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

/*
ObservePrice records timestamped price and derives returns so lead-lag
features use aligned observations.
*/
func (section *Section) ObservePrice(symbol string, price float64, at time.Time) bool {
	if symbol == "" || price <= 0 || at.IsZero() {
		return false
	}

	state := section.ensure(symbol)

	if state == nil {
		return false
	}

	if !state.lastSampleAt.IsZero() && !at.After(state.lastSampleAt) {
		return false
	}

	if !state.lastSampleAt.IsZero() {
		spacing := seriesSampleSpacing(state.prices, nil)

		if spacing > 0 && at.Sub(state.lastSampleAt) < spacing {
			return false
		}
	}

	state.last = price
	state.lastSampleAt = at
	state.observedCount++
	state.prices = append(state.prices, priceSample{at: at, value: price})

	capacity := priceRetentionCount(state.observedCount)

	if len(state.prices) > capacity {
		state.prices = state.prices[len(state.prices)-capacity:]
	}

	if symbol == section.anchorSymbol {
		section.recordAnchorMove(state.prices)
	}

	return true
}

/*
CausalAnchor selects the strongest return already present before the next frame
is ingested. The median cohort magnitude is the empirical null scale, so the
current frame can never choose its own explanatory anchor.
*/
func (section *Section) CausalAnchor() string {
	type candidate struct {
		symbol    string
		magnitude float64
	}
	candidates := make([]candidate, 0)
	magnitudes := make([]float64, 0)

	section.universe.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		state, stateOK := value.(*symbolState)

		if !symbolOK || !stateOK || state == nil || len(state.prices) < sampleFloor {
			return true
		}

		previous := state.prices[len(state.prices)-2].value
		latest := state.prices[len(state.prices)-1].value

		if previous <= 0 || latest <= 0 {
			return true
		}

		magnitude := math.Abs(math.Log(latest / previous))
		candidates = append(candidates, candidate{symbol: symbol, magnitude: magnitude})
		magnitudes = append(magnitudes, magnitude)

		return true
	})

	median, ok := statistic.MedianOf(magnitudes)

	if !ok {
		return ""
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].magnitude == candidates[right].magnitude {
			return candidates[left].symbol < candidates[right].symbol
		}

		return candidates[left].magnitude > candidates[right].magnitude
	})

	if len(candidates) == 0 || candidates[0].magnitude <= median {
		return ""
	}

	return candidates[0].symbol
}

/*
anchorState returns active anchor state so feature calculation reads one
explicit reference series.
*/
func (section *Section) anchorState() *symbolState {
	return section.ensure(section.anchorSymbol)
}

/*
Features derives lag and contemporaneous evidence for a symbol against the
current anchor.
*/
func (section *Section) Features(scope string) LagFeatures {
	anchor := section.anchorState()
	follower := section.ensure(scope)

	if anchor == nil || follower == nil {
		return LagFeatures{}
	}

	price := follower.last

	if scope == section.anchorSymbol {
		price = anchor.last
	}

	move := section.anchorMove()
	features := LagFeatures{
		IsAnchor:    scope == section.anchorSymbol,
		Price:       price,
		MoveReady:   move.ready,
		MoveMoved:   move.moved,
		StallMargin: move.stallMargin,
		ObservedAt:  follower.lastSampleAt,
	}

	if features.IsAnchor {
		if len(anchor.prices) > 0 {
			features.ObservedFrom = anchor.prices[0].at
		}

		return features
	}

	anchorSeries := append([]priceSample(nil), anchor.prices...)
	followerSeries := append([]priceSample(nil), follower.prices...)

	if len(anchorSeries) == 0 || len(followerSeries) == 0 {
		return features
	}

	observedFrom := anchorSeries[0].at

	if followerSeries[0].at.After(observedFrom) {
		observedFrom = followerSeries[0].at
	}

	features.ObservedFrom = observedFrom
	sampleCount := len(anchorSeries)

	if len(followerSeries) < sampleCount {
		sampleCount = len(followerSeries)
	}

	features.SampleCount = sampleCount

	anchorSamples := correlationSamples(anchorSeries)
	followerSamples := correlationSamples(followerSeries)
	sampleSpacing := seriesSampleSpacing(anchorSeries, followerSeries)

	if sampleSpacing <= 0 || absoluteDuration(
		anchor.lastSampleAt.Sub(follower.lastSampleAt),
	) > sampleSpacing {
		return features
	}

	contempCorr, contempOK := algorithm.HayashiPairCorrelation(
		anchorSamples,
		followerSamples,
		0,
	)
	features.ContempOK = contempOK
	features.ContempCorr = contempCorr

	lagBars, lagCorr, lagOK := algorithm.CrossLagScore(
		anchorSamples,
		followerSamples,
		sampleSpacing,
	)
	features.LagOK = lagOK
	features.LagBars = lagBars
	features.LagCorr = lagCorr

	if features.ObservedAt.IsZero() {
		features.ObservedAt = anchor.lastSampleAt
	}

	return features
}

func absoluteDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}

	return duration
}

/*
anchorMove records one observed anchor displacement so delayed responses can
be measured against its direction and time.
*/
type anchorMove struct {
	moved       bool
	stallMargin float64
	ready       bool
}

/*
recordAnchorMove records the latest significant anchor move so lag response is
evaluated against observed leadership.
*/
func (section *Section) recordAnchorMove(samples []priceSample) {
	if len(samples) < sampleFloor {
		return
	}

	window := samples[len(samples)-1].at.Sub(samples[0].at)
	recentMove, ok := recentPathMove(samples, window)

	if !ok {
		return
	}

	output, err := section.moveBaseline.Measure(recentMove)

	if err != nil {
		return
	}

	section.lastMove = output
}

/*
anchorMove returns the retained anchor move used to evaluate delayed symbol
response.
*/
func (section *Section) anchorMove() anchorMove {
	return anchorMove{
		moved:       section.lastMove.Moved > 0,
		stallMargin: section.lastMove.StallMargin,
		ready:       section.lastMove.Ready > 0,
	}
}

/*
maxLagBars derives supported lag depth from available samples so search never
exceeds observed evidence.
*/
func (section *Section) maxLagBars(sampleCount int) int {
	return resolvedMaxLagBars(sampleCount)
}

func correlationSamples(samples []priceSample) []correlation.Sample {
	converted := make([]correlation.Sample, len(samples))

	for index, sample := range samples {
		converted[index] = correlation.Sample{
			At:    sample.at,
			Value: sample.value,
		}
	}

	return converted
}

func newMoveBaseline() *algorithm.MoveBaseline {
	shortWindow, longWindow, err := statistic.ResolveWindows(
		[]float64{1, 1},
		0,
		0,
	)

	if err != nil {
		panic(err)
	}

	return algorithm.NewMoveBaseline(algorithm.MoveBaselineConfig{
		MinObs:  max(shortWindow, sampleFloor),
		PathCap: longWindow + shortWindow + 1,
	})
}

func priceRetentionCount(observedCount int) int {
	if observedCount <= 1 {
		return observedCount
	}

	_, longWindow, returnLag := windowsFromCount(observedCount)
	retention := longWindow + returnLag + 1

	if retention > observedCount {
		return observedCount
	}

	return retention
}

func resolvedMaxLagBars(sampleCount int) int {
	if sampleCount <= 0 {
		return 1
	}

	_, longWindow, _ := windowsFromCount(sampleCount)
	halfSeries := sampleCount / 2

	if longWindow > halfSeries {
		longWindow = halfSeries
	}

	if longWindow < 1 {
		longWindow = 1
	}

	return longWindow
}

func resolvedShortWindow(sampleCount int) int {
	if sampleCount <= 0 {
		return sampleFloor
	}

	shortWindow, _, _ := windowsFromCount(sampleCount)

	return max(shortWindow, sampleFloor)
}

func recentPathMove(samples []priceSample, window time.Duration) (float64, bool) {
	minSamples := resolvedShortWindow(len(samples))

	if len(samples) < minSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.at.Add(-window)
	startIndex := -1

	for index, sample := range samples {
		if !sample.at.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.value <= 0 || latest.value <= 0 {
		return 0, false
	}

	if start.value == latest.value {
		return 0, true
	}

	spacing := seriesSampleSpacing(samples, nil)

	if spacing <= 0 || minSamples < sampleFloor {
		return 0, false
	}

	minimumSpan := spacing * time.Duration(minSamples-1)

	if latest.at.Sub(start.at) < minimumSpan {
		return 0, false
	}

	return math.Abs(math.Log(latest.value / start.value)), true
}

func seriesSampleSpacing(primary, secondary []priceSample) time.Duration {
	spacing := medianSampleSpacing(primary)

	if len(secondary) > 1 {
		alternate := medianSampleSpacing(secondary)

		if alternate > 0 && (spacing <= 0 || alternate < spacing) {
			spacing = alternate
		}
	}

	if spacing <= 0 {
		return 0
	}

	return spacing
}

func medianSampleSpacing(samples []priceSample) time.Duration {
	if len(samples) < sampleFloor {
		return 0
	}

	gaps := make([]float64, 0, len(samples)-1)

	for index := 1; index < len(samples); index++ {
		gap := samples[index].at.Sub(samples[index-1].at).Seconds()

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	median, _ := statistic.MedianOf(gaps)

	return time.Duration(median * float64(time.Second))
}
