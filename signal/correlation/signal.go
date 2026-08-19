package correlation

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
historyCapacity bounds the rolling per-symbol return history the cohort sample
retains; it is also the denominator of measurement maturity.
*/
const historyCapacity = 128

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	algo       *algorithm.CohortSample
	classifier *equation.Cohort

	// CohortSample keeps an unsynchronized internal map, and the measurement
	// loop fans one goroutine per dirty symbol into this single signal. The
	// signal is the sole owner of the sampler, so it defends its own shared
	// state here rather than trusting the dependency to be thread-safe.
	algoMu sync.Mutex
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		algo: algorithm.NewCohortSample(algorithm.CohortSampleConfig{
			HistoryCap: historyCapacity,
		}),
		classifier: equation.NewCohort(),
	}
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

func (signal *Signal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) error {
	if symbol == nil {
		return nil
	}

	for ticker := range symbol.MarketTickers(types.SourceCorrelation) {
		// The cohort sampler needs the observed traded price to build
		// per-symbol return history; Change is a signed delta, not a
		// price, and fails the positive-price validation.
		price := ticker.Last.Float64()

		if price <= 0 {
			continue
		}

		signal.algoMu.Lock()
		output, ready, err := signal.algo.Measure(algorithm.CohortSampleInput{
			Symbol: ticker.Symbol,
			At:     ticker.Timestamp,
			Price:  price,
		})
		signal.algoMu.Unlock()

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"correlation: failed to measure ticker",
				err,
			))

			return nil
		}

		// The classifier only speaks once the cohort sampler has a complete
		// peer window; before that the schema is genuinely incomplete and a
		// zero-eligibility outcome is the correct representation, not an
		// error to log per tick. Once ready, an ineligible frame is also a
		// plain reading from equation.Cohort (zero outcome, no error); only
		// a malformed schema is a genuine failure worth surfacing.
		outcome := equation.CohortOutput{}

		if ready {
			classified, classifyErr := signal.classifier.Measure(output)

			if classifyErr != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"correlation: classifier rejected cohort frame",
					classifyErr,
				))

				return nil
			}

			outcome = classified
		}

		if err := symbol.AppendMeasurement(*signal.frame(ticker, price, output, outcome, ready)); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"correlation: failed to emit reading",
				err,
			))
		}
	}

	return nil
}

/*
frame materializes one ticker's cohort evidence as a measurement. The zero
classifier output carries zero maturity, so an immature window is distinguishable
from a classified one without either going dark.
*/
func (signal *Signal) frame(
	ticker kraken.TickerData,
	price float64,
	batch equation.FeatureFrame,
	outcome equation.CohortOutput,
	ready bool,
) *types.Measurement {
	maturity := 0.0

	if ready && outcome.Eligible && len(batch.Features) > 0 {
		maturity = batch.Features[0] / historyCapacity
	}

	metrics := map[string]types.MetricSample{
		types.MetricKey(types.MetricCorrelation, types.SideNone): {
			Raw:        outcome.Correlation,
			Normalized: &outcome.Correlation,
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricHerdScore, types.SideNone): {
			Raw:        outcome.HerdScore,
			Normalized: &outcome.HerdScore,
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricAlphaScore, types.SideNone): {
			Raw:        outcome.AlphaScore,
			Normalized: &outcome.AlphaScore,
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricNoiseScore, types.SideNone): {
			Raw:        outcome.NoiseScore,
			Normalized: &outcome.NoiseScore,
			Unit:       types.UnitDimensionless,
		},
		types.MetricKey(types.MetricStressScore, types.SideNone): {
			Raw:        outcome.StressScore,
			Normalized: &outcome.StressScore,
			Unit:       types.UnitDimensionless,
		},
	}

	return &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceCorrelation,
		Symbol:   ticker.Symbol,
		At:       ticker.Timestamp,
		Maturity: maturity,
		Metadata: map[string]float64{
			"price":    price,
			"energy":   outcome.Energy,
			"category": float64(outcome.Category),
		},
		Metrics: metrics,
	}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
