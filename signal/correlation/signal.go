package correlation

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	api     *websocket.API
	section *Section
	ui      chan []byte
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		section: NewSection(),
		ui:      ui,
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCorrelation)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceCorrelation
}

func (signal *Signal) Measure(market *types.Symbol, _ ...int64) []*types.Measurement {
	scoresBySymbol, err := signal.section.Measure(
		market.MarketTickers(types.SourceCorrelation),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "correlation: failed to measure tickers", err,
		))

		return nil
	}

	if len(scoresBySymbol) == 0 {
		return nil
	}

	scores, found := scoresBySymbol[market.Symbol]

	if !found {
		return nil
	}

	at, price, found := signal.section.Latest(market.Symbol)

	if !found {
		return nil
	}

	metrics, valid := correlationMetrics(scores)

	if !valid {
		return nil
	}

	measurement := &types.Measurement{
		ID:     uuid.NewString(),
		Source: types.SourceCorrelation,
		Symbol: market.Symbol,
		Tick:   market.Tick,
		At:     at,
		Metadata: map[string]float64{
			"last_price": price,
		},
		Metrics: metrics,
	}
	measurement.PutMetric(
		types.MetricLastPrice,
		types.SideNone,
		types.MetricSample{Raw: price, Unit: types.UnitQuoteCurrency},
	)
	separation, separationReady := types.MeasurementHypothesisSeparation(
		types.SourceCorrelation,
		measurement.Metrics,
	)

	if !separationReady {
		panic("correlation: competing metric groups are not measurable")
	}

	measurement.PutMetric(types.MetricHypothesisSeparation, types.SideNone, types.MetricSample{
		Raw: separation, Normalized: &separation, Unit: types.UnitDimensionless,
	})

	return []*types.Measurement{measurement}
}

/*
correlationMetrics maps the complete equation output onto measurement keys.
The equation already returns dimensionless scores and ratios, so Normalized
retains those values without applying a second transformation.
*/
func correlationMetrics(
	scores map[string]float64,
) (map[string]types.MetricSample, bool) {
	type reading struct {
		name   string
		metric types.MetricType
	}

	readings := []reading{
		{"correlation", types.MetricCorrelation},
		{"signed", types.MetricSigned},
		{"relativeEnergy", types.MetricRelativeEnergy},
		{"herdScore", types.MetricHerdScore},
		{"alphaScore", types.MetricAlphaScore},
		{"noiseScore", types.MetricNoiseScore},
		{"stressScore", types.MetricStressScore},
	}

	metrics := make(map[string]types.MetricSample, len(readings))
	valid := true

	for _, item := range readings {
		raw, exists := scores[item.name]
		var normalized *float64
		domainValid := exists

		if item.metric == types.MetricSigned {
			domainValid = domainValid && raw >= -1 && raw <= 1
		}

		if item.metric == types.MetricRelativeEnergy {
			domainValid = domainValid && raw >= 0
		}

		if item.metric != types.MetricSigned &&
			item.metric != types.MetricRelativeEnergy {
			domainValid = domainValid && raw >= 0 && raw <= 1
		}

		if !domainValid {
			valid = false
		} else {
			normalized = &raw
		}

		metrics[types.MetricKey(item.metric, types.SideNone)] = types.MetricSample{
			Raw:        raw,
			Normalized: normalized,
			Unit:       types.UnitDimensionless,
		}
	}

	return metrics, valid
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.section != nil {
		signal.section.Close()
	}

	return nil
}
