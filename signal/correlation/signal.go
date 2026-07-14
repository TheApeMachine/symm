package correlation

import (
	"context"

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

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		ticker:  NewTicker(ctx, api),
		section: NewSection(),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows := signal.ticker.cache
	out := make([]*types.Measurement, 0, len(rows))

	thesis.CrossSection.ProcessUpdates(rows)

	for _, row := range rows {
		scores, ok := signal.section.Scores(row.Symbol, thesis.CrossSection)

		if !ok {
			continue
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		out = append(out,
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricCorrelation, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["correlation"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricSigned, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["signed"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricRelativeEnergy, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["relativeEnergy"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricHerdScore, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["herdScore"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricAlphaScore, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["alphaScore"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricNoiseScore, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["noiseScore"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricStressScore, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["stressScore"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricPeakScore, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["peakScore"], Validity: validity},
			&types.Measurement{Source: types.SourceCorrelation, Metric: types.MetricStrength, Stream: types.Correlation, Symbol: row.Symbol, At: row.Timestamp, Unit: types.UnitDimensionless, Raw: scores["strength"], Validity: validity},
		)
	}

	signal.ticker.cache = signal.ticker.cache[:0]

	thesis.Signals.Store("tickers", rows)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
