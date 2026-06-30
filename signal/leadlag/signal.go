package leadlag

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
LeadLag is the "Anchor" perspective, measuring the temporal correlation
between the current cross-section leader (the pair the universe is chasing,
derived live via CrossSection.Leader — no config major) and each follower.

# Summary of LeadLag Categories

| Category           | Lead/Lag Correlation | Lag Fraction | Market "Feel"             |
|:-------------------|:---------------------|:-------------|:--------------------------|
| Inefficient Lag    | High                 | High         | Catch-up Opportunity      |
| Synchronized Drift | High                 | Low          | Systemic Beta             |
| Decoupled Move     | Low                  | N/A          | Idiosyncratic Alpha       |
| Anchor Stall       | Low                  | Low          | Leadership Exhaustion     |
*/
/*
Signal measures temporal correlation between the anchor pair and each follower.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	tree    *dmt.Tree
	Section *Section
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		tree:    tree,
		Section: NewSection(),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil || signal.Section == nil || crossSection == nil {
			return
		}

		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		// Anchor is the live cross-section leader — the pair the universe is
		// chasing this cycle — not a config major. No leader, no lead-lag story.
		anchor := crossSection.Leader()

		if anchor == "" {
			return
		}

		signal.Section.SetAnchor(anchor)

		for rowIndex := 0; ; rowIndex++ {
			row, rowErr := market.SymbolFromTicker(datapoint, rowIndex)

			if rowErr != nil {
				return
			}

			signal.Section.ObservePrice(row.Name, row.Price, row.Updated)

			features := signal.Section.Features(row.Name)

			if features.Price <= 0 {
				continue
			}

			lagFraction := 0.0
			lagCorrelation := 0.0
			contempCorrelation := 0.0

			if features.LagOK && features.SampleCount > 0 {
				dynamicMax := signal.Section.maxLagBars(features.SampleCount)

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

			// Co-movement (a resolved contemporaneous or lagged correlation) is
			// itself anchor activity: on the first observation, before any move
			// history has matured, an honest correlation must still carry low-
			// confidence mass rather than being zeroed behind a move warmup.
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

			shares := []dist.Share{
				{Key: "inefficient", Category: logic.CategoryInefficientLag, Mass: sampleSupport * anchorActive * (lagWeight + stallWeight) * (lagCorrelation + stallWeight) * (1 + lagCorrelation + stallWeight)},
				{Key: "sync", Category: logic.CategorySynchronizedDrift, Mass: sampleSupport * contempCorrelation * (1 - lagFraction) * anchorActive * (1 - stallWeight)},
				{Key: "decoupled", Category: logic.CategoryDecoupledMove, Mass: sampleSupport * (1 - correlation) * anchorActive * math.Pow(1-lagFraction, lagDampExponent) * (1 - lagCorrelation) * (1 - stallWeight)},
				{Key: "stall", Category: logic.CategoryAnchorStall, Mass: sampleSupport * (1 - correlation) * features.StallMargin * (1 - lagFraction) * stallDamp},
			}

			measurement := datura.Acquire("leadlag", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(row.Name)
			errnie.Error(measurement.SetOrigin(string(logic.SourceLeadLag)))
			measurement.SetTimestamp(datapoint.Timestamp())

			measurement.MergeOutput("correlation", correlation)
			measurement.MergeOutput("lagFraction", lagFraction)
			measurement.MergeOutput("sampleSupport", sampleSupport)
			confidence := dist.Write(measurement, shares)

			if confidence <= 0 {
				measurement.Release()
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
