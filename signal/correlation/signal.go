package correlation

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
)

/*
Correlation is the "Herd Behavior" perspective, measuring synchronized return
correlation across the subscribed universe using a rolling window of log-returns.

# Summary of Correlation Categories

| Category          | Correlation Level       | Variance | Market "Feel"                           |
|:------------------|:------------------------|:---------|:----------------------------------------|
| Systemic Herd     | High (adaptive quantile)| High     | Global Beta / Momentum Drift            |
| Decoupled Alpha   | Low                     | High     | Unique Driver / Leading Move            |
| Stochastic Noise  | Low                     | Low      | Quiet / Indecisive                      |
| Divergent Stress  | Negative                | High     | Contrarian Move / Relative Weakness     |
*/
/*
Signal measures how each symbol's return stream correlates with the cross-section median.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if signal == nil || datapoint == nil || crossSection == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "ticker" {
		return nil
	}

	row, rowErr := market.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(crossSection.Observe(row)) != nil {
		return nil
	}

	window := crossSection.MinBarsRequired()
	correlation, energy, peerCorrelations, peerEnergyMedian, ok := crossSection.SymbolPeerStats(row.Name, window)

	if !ok {
		return nil
	}

	decoupling := math.Max(0, 1-math.Abs(correlation))
	relativeEnergy := energy / (energy + peerEnergyMedian)

	herdGate := 0.0

	if len(peerCorrelations) > 0 {
		percentile := peerHerdingPercentile(peerCorrelations)

		if gate, gateErr := peerQuantile(peerCorrelations, percentile); gateErr != nil {
			errnie.Error(errnie.Err(errnie.Validation, "correlation: herd gate quantile", gateErr))
		} else {
			herdGate = gate
		}
	}
	herd := math.Max(0, correlation-herdGate) * energy
	herdAligned := math.Max(0, correlation) * (1 - decoupling) * energy

	if herdAligned > herd {
		herd = herdAligned
	}

	herd *= 1 - decoupling*decoupling

	alpha := decoupling * decoupling * relativeEnergy * energy * (1 + relativeEnergy)
	noise := decoupling * (1 - decoupling) * (1 - relativeEnergy) / (1 + energy)
	stress := math.Max(0, -correlation) * energy

	shares := []dist.Share{
		{Key: "herdScore", Category: logic.CategorySystemicHerd, Mass: herd},
		{Key: "alphaScore", Category: logic.CategoryDecoupledAlpha, Mass: alpha},
		{Key: "noiseScore", Category: logic.CategoryStochasticNoise, Mass: noise},
		{Key: "stressScore", Category: logic.CategoryDivergentStress, Mass: stress},
	}

	measurement := datura.Acquire("correlation", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(row.Name)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCorrelation)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("correlation", correlation)
	measurement.MergeOutput("energy", energy)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func peerHerdingPercentile(peerCorrelations []float64) float64 {
	lower, upper, err := statutil.Quartiles(peerCorrelations)

	if err != nil {
		return 0.5
	}

	median := statutil.Median(peerCorrelations)
	span := upper - lower

	if span <= 0 && median == 0 {
		return 0.5
	}

	return span / (math.Abs(median) + span)
}

func peerQuantile(values []float64, percentile float64) (float64, error) {
	filtered := make([]float64, 0, len(values))

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		filtered = append(filtered, value)
	}

	if len(filtered) == 0 {
		return 0, fmt.Errorf("correlation: peer quantile requires finite samples")
	}

	return statutil.Quantile(percentile, filtered)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
