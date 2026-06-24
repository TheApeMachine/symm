package leadlag

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	marketsection "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
LeadLag is the "Anchor" perspective, measuring the temporal correlation
between a leader asset (config: market.anchor_symbol, default BTC/USD) and
each follower.

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
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section, sectionErr := NewSectionFromConfig()

	if sectionErr != nil || section == nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		tree:    tree,
		Section: section,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.Section == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "ticker" {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	signal.Section.ObservePrice(row.Name, row.Price, row.Updated)

	features := signal.Section.Features(row.Name)

	if features.Price <= 0 {
		return nil
	}

	lagFraction := 0.0
	lagCorrelation := 0.0
	contempCorrelation := 0.0

	if features.LagOK && maxLagBars > 0 {
		lagFraction = float64(features.LagBars) / float64(maxLagBars)
		lagCorrelation = math.Abs(features.LagCorr)
	}

	if features.ContempOK {
		contempCorrelation = math.Abs(features.ContempCorr)
	}

	correlation := math.Max(contempCorrelation, lagCorrelation)

	anchorActive := 0.0

	if features.MoveMoved || (features.StallMargin > 0 && lagFraction > 0) {
		anchorActive = 1
	}

	stallWeight := features.StallMargin * (1 + features.StallMargin)
	lagWeight := lagFraction + lagFraction*lagFraction
	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	shares := []dist.Share{
		{Key: "inefficient", Category: logic.CategoryInefficientLag, Mass: anchorActive * (lagWeight + stallWeight) * (lagCorrelation + stallWeight) * (1 + lagCorrelation + stallWeight)},
		{Key: "sync", Category: logic.CategorySynchronizedDrift, Mass: contempCorrelation * (1 - lagFraction) * anchorActive * (1 - stallWeight)},
		{Key: "decoupled", Category: logic.CategoryDecoupledMove, Mass: (1 - correlation) * anchorActive * math.Pow(1-lagFraction, 8) * (1 - lagCorrelation) * (1 - stallWeight)},
		{Key: "stall", Category: logic.CategoryAnchorStall, Mass: (1 - correlation) * features.StallMargin * (1 - lagFraction) * stallDamp},
	}

	measurement := datura.Acquire("leadlag", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(row.Name)
	errnie.Error(measurement.SetOrigin(string(logic.SourceLeadLag)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("correlation", correlation)
	measurement.MergeOutput("lagFraction", lagFraction)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
