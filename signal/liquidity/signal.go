package liquidity

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Scarcity perspective, identifying opportunities where current
executable touch depth is thin relative to peers. Reported-volume notional is
retained as a separate turnover context and never mixed into the book-depth score.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	ui     chan []byte
}

/*
NewSignal creates liquidity measurement state for central market cuts so each
tick can compare executable liquidity across the observed cohort.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires ticker depth fields for cross-sectional scarcity.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamTicker
}

/*
Measure returns typed measurements for the cut, or an error when the
cut cannot be measured honestly.
*/
func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, error) {
	return signal.Calculate(thesis.Market())
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
	crossSection := frame.CrossSection
	notionalPeers := make([]float64, 0)
	depthPeers := make([]float64, 0)

	crossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)

		if metric.QuoteNotional > 0 {
			notionalPeers = append(notionalPeers, metric.QuoteNotional)
		}

		if metric.ExecutableDepth > 0 {
			depthPeers = append(depthPeers, metric.ExecutableDepth)
		}

		return true
	})

	depthMedian, depthOK := statistic.MedianOf(depthPeers)
	peerReady := len(depthPeers) >= 2 && depthOK && depthMedian > 0
	notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)
	peerMaturity := float64(len(depthPeers)) / float64(len(depthPeers)+1)

	type measurementSpec struct {
		metric     types.MetricType
		unit       types.MeasurementUnit
		raw        float64
		normalized *float64
	}

	for _, row := range rows {
		executableDepth := crossSection.ExecutableDepth(row)

		if executableDepth <= 0 {
			continue
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}
		scale := types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    row.Timestamp,
			Through: row.Timestamp,
		}
		specs := []measurementSpec{
			{types.MetricExecutableTouchDepth, types.UnitQuoteCurrency, executableDepth, nil},
		}

		if !peerReady {
			validity.State = types.ValidityProvisional
			validity.Reason = "peer executable-depth median unavailable"
		}

		if peerReady {
			relativeDepth := executableDepth / depthMedian
			scarcity := math.Max(0, 1-relativeDepth)
			specs = append(specs,
				measurementSpec{types.MetricRelativeTouchDepth, types.UnitDimensionless, relativeDepth, types.NormalizeFinite(relativeDepth)},
				measurementSpec{types.MetricScarcityScore, types.UnitDimensionless, scarcity, types.NormalizeFinite(scarcity)},
				measurementSpec{types.MetricExecutableTouchDepthMedian, types.UnitQuoteCurrency, depthMedian, nil},
			)
		}

		reportedNotional := crossSection.QuoteNotional(row)

		if peerReady && reportedNotional > 0 && hasNotionalMedian && notionalMedian > 0 {
			specs = append(specs,
				measurementSpec{types.MetricReportedVolumeNotional, types.UnitQuoteCurrency, reportedNotional, nil},
				measurementSpec{types.MetricReportedVolumeNotionalMedian, types.UnitQuoteCurrency, notionalMedian, nil},
			)
		}

		for _, spec := range specs {
			measurements := []*types.Measurement{
				{
					Source:     types.SourceLiquidity,
					Stream:     types.Liquidity,
					Metric:     spec.metric,
					Subject:    types.SubjectPeerLiquidity,
					Symbol:     row.Symbol,
					At:         row.Timestamp,
					Unit:       spec.unit,
					Raw:        spec.raw,
					Normalized: spec.normalized,
					Maturity:   peerMaturity,
					Validity:   validity,
					Scale:      scale,
				},
			}

			out = append(out, measurements...)
		}
	}

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
