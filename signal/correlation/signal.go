package correlation

import (
	"context"
	"strings"
	"time"

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
	ticker  *Ticker
	section *Section
}

/*
NewSignal creates correlation measurement state and subscribes its ticker
input so successive ticks can establish real price relationships.
*/
func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		ticker:  NewTicker(ctx, api),
		section: NewSection(),
	}
}

/*
Capture freezes the ticker journal so the asynchronous price paths consume a
complete ingress range from one planner cycle.
*/
func (signal *Signal) Capture(at time.Time) error {
	return signal.ticker.cache.Capture(at)
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows, err := signal.ticker.cache.Drain(thesis.At)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"correlation: ticker drain failed",
			err,
		))
		return thesis
	}

	out := make([]*types.Measurement, 0, len(rows))

	thesis.CrossSection.Measure(rows)
	scoresBySymbol := signal.section.Measure(rows)
	latestAtBySymbol := make(map[string]time.Time, len(rows))

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if !row.Timestamp.After(latestAtBySymbol[symbol]) {
			continue
		}

		latestAtBySymbol[symbol] = row.Timestamp
	}

	for _, metric := range thesis.CrossSection.Metrics {
		scores, ok := scoresBySymbol[metric.Symbol]

		if !ok {
			continue
		}

		at, ok := latestAtBySymbol[metric.Symbol]

		if !ok || at.IsZero() {
			continue
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		out = append(out,
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricCorrelation,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["correlation"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricSigned,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["signed"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricRelativeEnergy,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["relativeEnergy"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricHerdScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["herdScore"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricAlphaScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["alphaScore"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricNoiseScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["noiseScore"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStressScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["stressScore"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricPeakScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["peakScore"],
				Validity: validity,
			},
			&types.Measurement{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStrength,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["strength"],
				Validity: validity,
			},
		)
	}

	thesis.Signals.Store("correlation.tickers", rows)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
