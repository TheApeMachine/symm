package leadlag

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/correlation"
)

const (
	priceHistoryCap   = 256
	minLagSamples     = 16
	maxLagBars        = 12
	anchorMoveMinObs  = 12
	barInterval       = 5 * time.Minute
	ringSampleSpacing = 15 * time.Second
)

/*
Section tracks anchor-relative price paths for lead-lag scoring.
*/
type Section struct {
	universe       sync.Map
	anchorBaseline *algorithm.MoveBaseline
	anchorSymbol   string
}

/*
LagFeatures holds encoded inputs for the lag algorithm stage.
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

type symbolState struct {
	last         float64
	lastSampleAt time.Time
	prices       []correlation.Sample
}

/*
NewSection allocates a lead-lag cross section for one anchor symbol.
*/
func NewSection(anchorSymbol string) *Section {
	resolved := anchorSymbol

	if resolved == "" {
		resolved = "BTC/USD"
	}

	return &Section{
		anchorSymbol:   resolved,
		anchorBaseline: algorithm.NewMoveBaseline(anchorMoveMinObs, priceHistoryCap),
	}
}

/*
NewSectionFromConfig loads the anchor symbol from market config.
*/
func NewSectionFromConfig() (*Section, error) {
	return NewSection("BTC/USD"), nil
}

func (section *Section) AnchorSymbol() string {
	return section.anchorSymbol
}

func (section *Section) ensure(symbol string) *symbolState {
	raw, _ := section.universe.LoadOrStore(symbol, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

/*
ObservePrice records one price sample for a symbol.
*/
func (section *Section) ObservePrice(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 || at.IsZero() {
		return
	}

	state := section.ensure(symbol)

	if state == nil {
		return
	}

	state.last = price

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices = append(state.prices, correlation.Sample{At: at, Value: price})

	if len(state.prices) > priceHistoryCap {
		state.prices = state.prices[len(state.prices)-priceHistoryCap:]
	}
}

func (section *Section) anchorState() *symbolState {
	return section.ensure(section.anchorSymbol)
}

/*
Features derives lag inputs for one scope symbol.
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
		return features
	}

	anchorSeries := append([]correlation.Sample(nil), anchor.prices...)
	followerSeries := append([]correlation.Sample(nil), follower.prices...)
	sampleCount := len(anchorSeries)

	if len(followerSeries) < sampleCount {
		sampleCount = len(followerSeries)
	}

	features.SampleCount = sampleCount

	contempCorr, contempOK := algorithm.HayashiPairCorrelation(anchorSeries, followerSeries, 0)
	features.ContempOK = contempOK
	features.ContempCorr = contempCorr

	lagBars, lagCorr, lagOK := algorithm.CrossLagScore(anchorSeries, followerSeries, barInterval)
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

func (section *Section) anchorMove() anchorMove {
	anchor := section.anchorState()

	if anchor == nil {
		return anchorMove{}
	}

	window := time.Duration(maxLagBars) * barInterval
	recentMove, ok := recentPathMove(anchor.prices, window)

	if !ok {
		return anchorMove{}
	}

	moved, stallMargin, ready := section.anchorBaseline.Evaluate(recentMove)

	return anchorMove{moved: moved, stallMargin: stallMargin, ready: ready}
}

func recentPathMove(samples []correlation.Sample, window time.Duration) (float64, bool) {
	if len(samples) < minLagSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.At.Add(-window)
	startIndex := -1

	for index, sample := range samples {
		if !sample.At.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.Value <= 0 || latest.Value <= 0 {
		return 0, false
	}

	minimumSpan := ringSampleSpacing * time.Duration(minLagSamples-1)

	if latest.At.Sub(start.At) < minimumSpan {
		return 0, false
	}

	return math.Abs(math.Log(latest.Value / start.Value)), true
}

func (section *Section) PriceSamples(symbol string) []correlation.Sample {
	state := section.ensure(symbol)

	if state == nil {
		return nil
	}

	return append([]correlation.Sample(nil), state.prices...)
}

func (section *Section) LastPrice(symbol string) float64 {
	state := section.ensure(symbol)

	if state == nil {
		return 0
	}

	return state.last
}
