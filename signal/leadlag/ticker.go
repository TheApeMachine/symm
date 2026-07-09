package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	section    *Section
	classifier *probability.ScoreClassifier
}

func NewTicker(section *Section) *Ticker {
	return &Ticker{
		section: section,
		classifier: probability.NewScoreClassifier(
			[]string{"inefficient", "sync", "decoupled", "stall"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryInefficientLag)),
				float64(types.CategoryIndex(types.CategorySynchronizedDrift)),
				float64(types.CategoryIndex(types.CategoryDecoupledMove)),
				float64(types.CategoryIndex(types.CategoryAnchorStall)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Err(errnie.Validation, "leadlag: cross-section required", nil)
	}

	if row.Timestamp.IsZero() {
		return nil, errnie.Err(errnie.UnprocessableContent, "leadlag: event timestamp required", nil)
	}

	// Anchor is the live cross-section leader — the pair the universe is
	// chasing this cycle — not a config major. No leader, no lead-lag story.
	anchor := crossSection.Leader()

	if anchor == "" {
		return nil, nil
	}

	ticker.section.SetAnchor(anchor)

	lastPrice := row.Last.Float64()

	if lastPrice <= 0 {
		return nil, errnie.Err(errnie.UnprocessableContent, "leadlag: ticker last price required", nil)
	}

	ticker.section.ObservePrice(row.Symbol, lastPrice, row.Timestamp)

	features := ticker.section.Features(row.Symbol)

	if features.Price <= 0 {
		return nil, nil
	}

	measurement, err := ticker.measurementFromFeatures(row.Symbol, row.Timestamp, features)

	if err != nil || measurement == nil {
		return nil, err
	}

	return []*types.Measurement{measurement}, nil
}

func (ticker *Ticker) measurementFromFeatures(
	symbol string,
	at time.Time,
	features LagFeatures,
) (*types.Measurement, error) {
	lagFraction := 0.0
	lagCorrelation := 0.0
	contempCorrelation := 0.0

	if features.LagOK && features.SampleCount > 0 {
		dynamicMax := ticker.section.maxLagBars(features.SampleCount)

		if dynamicMax > 0 {
			lagFraction = float64(features.LagBars) / float64(dynamicMax)
		}

		lagCorrelation = math.Abs(features.LagCorr)
	}

	if features.ContempOK {
		contempCorrelation = math.Abs(features.ContempCorr)
	}

	correlation := math.Max(contempCorrelation, lagCorrelation)

	if correlation > 1 {
		correlation = 1
	}

	sampleSupport := 0.0

	if features.SampleCount > 0 {
		minSamples := minCorrelationSamples(features.SampleCount)

		if minSamples > 0 {
			sampleSupport = float64(features.SampleCount) / float64(minSamples)
		}
	}

	anchorActive := 0.0

	// Co-movement (a resolved contemporaneous or lagged correlation) is itself
	// anchor activity: once a relationship is measurable, emit low-confidence
	// evidence rather than zeroing behind a move warmup.
	if features.MoveMoved ||
		(features.StallMargin > 0 && lagFraction > 0) ||
		features.ContempOK ||
		features.LagOK {
		anchorActive = 1
	}

	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	stallMargin := math.Min(1, math.Max(0, features.StallMargin))
	noLag := 1 - lagFraction
	uncorrelated := 1 - correlation
	lagEvidence := lagCorrelation * lagFraction
	syncEvidence := contempCorrelation * noLag
	decoupledEvidence := uncorrelated * (1 - stallMargin)
	stallEvidence := stallMargin * uncorrelated * noLag * stallDamp

	inefficient := sampleSupport * anchorActive * lagEvidence * (1 - stallMargin)
	syncScore := sampleSupport * anchorActive * syncEvidence * (1 - stallMargin)
	decoupled := sampleSupport * anchorActive * decoupledEvidence
	stall := sampleSupport * anchorActive * stallEvidence

	for _, value := range []*float64{
		&correlation,
		&lagFraction,
		&sampleSupport,
		&inefficient,
		&syncScore,
		&decoupled,
		&stall,
	} {
		if math.IsNaN(*value) || math.IsInf(*value, 0) {
			*value = 0
		}
	}

	strength := max(max(inefficient, syncScore), max(decoupled, stall))

	if strength <= 0 {
		return nil, nil
	}

	result, err := ticker.classifier.Classify(map[string]float64{
		"inefficient": inefficient,
		"sync":        syncScore,
		"decoupled":   decoupled,
		"stall":       stall,
		"strength":    strength,
	})

	if err != nil {
		return nil, err
	}

	categories := []types.CategoryType{
		types.InefficientLag,
		types.SynchronizedDrift,
		types.DecoupledMove,
		types.AnchorStall,
	}
	strengths := []float64{
		inefficient,
		syncScore,
		decoupled,
		stall,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceLeadLag,
		Stream:        "ticker",
		Symbol:        symbol,
		At:            at,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"correlation":   correlation,
			"lagFraction":   lagFraction,
			"sampleSupport": sampleSupport,
			"inefficient":   inefficient,
			"sync":          syncScore,
			"decoupled":     decoupled,
			"stall":         stall,
			"strength":      strength,
		},
	}

	return measurement, nil
}
