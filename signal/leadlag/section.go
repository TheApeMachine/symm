package leadlag

import (
	"math"
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
	IsAnchor    bool
	Price       float64
	MoveReady   bool
	MoveMoved   bool
	StallMargin float64
	LagOK       bool
	LagBars     int
	LagCorr     float64
	ContempOK   bool
	ContempCorr float64
	SampleCount int
	ObservedAt  time.Time
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
PriceSampleCount returns how many spaced price samples are buffered for symbol.
*/
func (section *Section) PriceSampleCount(symbol string) int {
	state := section.ensure(symbol)

	if state == nil {
		return 0
	}

	return len(state.prices)
}

func (section *Section) ensure(symbol string) *symbolState {
	raw, _ := section.universe.LoadOrStore(symbol, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

func (section *Section) ObservePrice(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 || at.IsZero() {
		return
	}

	state := section.ensure(symbol)

	if state == nil {
		return
	}

	state.last = price

	if !state.lastSampleAt.IsZero() {
		spacing := seriesSampleSpacing(state.prices, nil)

		if spacing > 0 && at.Sub(state.lastSampleAt) < spacing {
			return
		}
	}

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
}

func (section *Section) anchorState() *symbolState {
	return section.ensure(section.anchorSymbol)
}

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
		return features
	}

	anchorSeries := append([]priceSample(nil), anchor.prices...)
	followerSeries := append([]priceSample(nil), follower.prices...)
	sampleCount := len(anchorSeries)

	if len(followerSeries) < sampleCount {
		sampleCount = len(followerSeries)
	}

	features.SampleCount = sampleCount

	anchorSamples := correlationSamples(anchorSeries)
	followerSamples := correlationSamples(followerSeries)
	sampleSpacing := seriesSampleSpacing(anchorSeries, followerSeries)

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

type anchorMove struct {
	moved       bool
	stallMargin float64
	ready       bool
}

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

func (section *Section) anchorMove() anchorMove {
	return anchorMove{
		moved:       section.lastMove.Moved > 0,
		stallMargin: section.lastMove.StallMargin,
		ready:       section.lastMove.Ready > 0,
	}
}

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
		shortWindow = sampleFloor
		longWindow = sampleFloor + 1
	}

	return algorithm.NewMoveBaseline(algorithm.MoveBaselineConfig{
		MinObs:  max(shortWindow, sampleFloor),
		PathCap: max(longWindow+shortWindow+1, 64),
	})
}

func priceRetentionCount(observedCount int) int {
	if observedCount <= 1 {
		return observedCount
	}

	windows, err := statistic.ResolveWindowSet(
		make([]float64, observedCount),
		statistic.WindowsConfig{},
	)

	if err != nil {
		return observedCount
	}

	retention := windows.LongWindow + windows.ReturnLag + 1

	if retention > observedCount {
		return observedCount
	}

	return retention
}

func resolvedMaxLagBars(sampleCount int) int {
	if sampleCount <= 0 {
		return 1
	}

	_, longWindow, err := statistic.ResolveWindows(
		make([]float64, sampleCount),
		0,
		0,
	)

	if err != nil {
		return 1
	}

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

	shortWindow, _, err := statistic.ResolveWindows(
		make([]float64, sampleCount),
		0,
		0,
	)

	if err != nil {
		return sampleFloor
	}

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
