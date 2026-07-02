package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

type Ticker struct {
	section *Section
}

func NewTicker(section *Section) *Ticker {
	return &Ticker{section: section}
}

func (ticker *Ticker) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if ticker == nil || ticker.section == nil || frame == nil || crossSection == nil {
		return nil
	}

	if frame.Timestamp() <= 0 {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"leadlag: event timestamp required",
			nil,
		)))
	}

	// Anchor is the live cross-section leader — the pair the universe is
	// chasing this cycle — not a config major. No leader, no lead-lag story.
	anchor := crossSection.Leader()

	if anchor == "" {
		return nil
	}

	ticker.section.SetAnchor(anchor)

	symbol, _ := frame.Scope()
	price := datura.Peek[float64](frame, "last")

	if price <= 0 {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"leadlag: ticker last price required",
			nil,
		)))
	}

	ticker.section.ObservePrice(symbol, price, time.Unix(0, frame.Timestamp()).UTC())

	features := ticker.section.Features(symbol)

	if features.Price <= 0 {
		return nil
	}

	return ticker.measurementFromFeatures(symbol, frame.Timestamp(), features)
}

func (ticker *Ticker) measurementFromFeatures(
	symbol string,
	timestamp int64,
	features LagFeatures,
) *datura.Artifact {
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

	stallWeight := features.StallMargin * (1 + features.StallMargin)
	lagWeight := lagFraction + lagFraction*lagFraction
	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	lagDampExponent := 1.0

	if features.LagOK {
		lagDampExponent = 1 + float64(features.LagBars)*features.LagCorr*(1+features.StallMargin)
	}

	measurement := datura.Acquire("leadlag", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceLeadLag)))
	measurement.SetTimestamp(timestamp)
	measurement.MergeOutput("correlation", correlation)
	measurement.MergeOutput("lagFraction", lagFraction)
	measurement.MergeOutput("sampleSupport", sampleSupport)

	if confidence := dist.Write(measurement, []dist.Share{
		{Key: "inefficient", Category: logic.CategoryInefficientLag, Mass: sampleSupport * anchorActive * (lagWeight + stallWeight) * (lagCorrelation + stallWeight) * (1 + lagCorrelation + stallWeight)},
		{Key: "sync", Category: logic.CategorySynchronizedDrift, Mass: sampleSupport * contempCorrelation * (1 - lagFraction) * anchorActive * (1 - stallWeight)},
		{Key: "decoupled", Category: logic.CategoryDecoupledMove, Mass: sampleSupport * (1 - correlation) * anchorActive * math.Pow(1-lagFraction, lagDampExponent) * (1 - lagCorrelation) * (1 - stallWeight)},
		{Key: "stall", Category: logic.CategoryAnchorStall, Mass: sampleSupport * (1 - correlation) * features.StallMargin * (1 - lagFraction) * stallDamp},
	}); confidence <= 0 {
		measurement.Release()
		return nil
	}

	return measurement
}
