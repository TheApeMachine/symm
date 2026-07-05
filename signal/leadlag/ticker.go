package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
				float64(logic.CategoryIndex(logic.CategoryInefficientLag)),
				float64(logic.CategoryIndex(logic.CategorySynchronizedDrift)),
				float64(logic.CategoryIndex(logic.CategoryDecoupledMove)),
				float64(logic.CategoryIndex(logic.CategoryAnchorStall)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	crossSection *market.CrossSection,
) (*logic.Measurement, error) {
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

	if row.Last <= 0 {
		return nil, errnie.Err(errnie.UnprocessableContent, "leadlag: ticker last price required", nil)
	}

	ticker.section.ObservePrice(row.Symbol, row.Last, row.Timestamp)

	features := ticker.section.Features(row.Symbol)

	if features.Price <= 0 {
		return nil, nil
	}

	return ticker.measurementFromFeatures(row.Symbol, row.Timestamp, features)
}

func (ticker *Ticker) measurementFromFeatures(
	symbol string,
	at time.Time,
	features LagFeatures,
) (*logic.Measurement, error) {
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

	measurement := logic.NewMeasurement(logic.SourceLeadLag, symbol, at)
	measurement.AddMetric("correlation", correlation)
	measurement.AddMetric("lagFraction", lagFraction)
	measurement.AddMetric("sampleSupport", sampleSupport)
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

	measurement.AddMetric("inefficient", inefficient)
	measurement.AddMetric("sync", syncScore)
	measurement.AddMetric("decoupled", decoupled)
	measurement.AddMetric("stall", stall)
	measurement.AddMetric("strength", strength)

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, err
	}

	if err := measurement.Ready(); err != nil {
		return nil, err
	}

	return measurement, nil
}
