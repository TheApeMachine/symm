package correlation

import (
	"context"
	"strings"
	"time"

	"github.com/theapemachine/datura"
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
	section *Section
	ui      chan []byte
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		section: NewSection(),
		ui:      ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires the ticker stream; correlation establishes price relationships
across the cross-sectional quote surface.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamTicker
}

/*
Measure supports direct replay against the legacy signal-local journal. The
live runtime uses Calculate with the central immutable market cut.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	rows := frame.Tickers
	out := make([]*types.Measurement, 0, len(rows))

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

	for _, metric := range frame.CrossSection.Metrics {
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

		measurements := []*types.Measurement{
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricCorrelation,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["correlation"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricSigned,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["signed"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricRelativeEnergy,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["relativeEnergy"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricHerdScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["herdScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricAlphaScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["alphaScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricNoiseScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["noiseScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStressScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["stressScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricPeakScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["peakScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStrength,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["strength"],
				Validity: validity,
			},
		}

		out = append(out, measurements...)
	}

	return out, nil
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
