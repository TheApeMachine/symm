package correlation

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	marketsection "github.com/theapemachine/symm/market"
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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	crossSection ...*marketsection.CrossSection,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section := (*marketsection.CrossSection)(nil)

	if len(crossSection) > 0 {
		section = crossSection[0]
	}

	if section == nil {
		var err error

		section, err = marketsection.NewCrossSection(marketsection.DefaultCrossSectionConfig())

		if err != nil {
			cancel()

			return nil
		}
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		tree:         tree,
		CrossSection: section,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.CrossSection == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "ticker" {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	window := signal.CrossSection.MinBarsRequired()
	correlation, energy, peerCorrelations, peerEnergyMedian, ok := signal.CrossSection.SymbolPeerStats(row.Name, window)

	if !ok {
		return nil
	}

	decoupling := math.Max(0, 1-math.Abs(correlation))
	relativeEnergy := energy / (energy + peerEnergyMedian)

	herdGate := 0.0

	if gate, gateErr := peerQuantile(peerCorrelations, 0.9); gateErr != nil {
		errnie.Error(errnie.Err(errnie.Validation, "correlation: herd gate quantile", gateErr))
	} else {
		herdGate = gate
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
