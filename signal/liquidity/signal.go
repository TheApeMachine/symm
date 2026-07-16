package liquidity

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities where current
executable touch depth is thin relative to peers. Reported-volume notional is
retained as separate turnover context and never mixed into the book-depth score.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	ticker *Ticker
}

/*
NewSignal creates liquidity measurement state and subscribes its ticker input
so each tick can compare executable liquidity across the observed cohort.
*/
func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(ctx, api),
	}
}

/*
Capture freezes the ticker journal before planner starts measuring signals so
the peer surface cannot change midway through one Thesis.
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
	rows, err := signal.ticker.cache.Frame(thesis.At)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"liquidity: ticker frame failed",
			err,
		))
		return thesis
	}

	out := make([]*types.Measurement, 0, len(rows))

	thesis.CrossSection.Measure(rows)

	metrics := thesis.CrossSection.Metrics
	notionalPeers := make([]float64, 0, len(metrics))
	depthPeers := make([]float64, 0, len(metrics))

	for _, metric := range metrics {
		if metric.QuoteNotional > 0 {
			notionalPeers = append(notionalPeers, metric.QuoteNotional)
		}

		if metric.ExecutableDepth > 0 {
			depthPeers = append(depthPeers, metric.ExecutableDepth)
		}
	}

	depthMedian, depthOK := statistic.MedianOf(depthPeers)

	if len(depthPeers) < 2 || !depthOK || depthMedian <= 0 {
		thesis.Signals.Store("liquidity.tickers", rows)
		return thesis
	}

	notionalMedian, hasNotionalMedian := statistic.MedianOf(notionalPeers)
	peerMaturity := float64(len(depthPeers)) / float64(len(depthPeers)+1)
	type measurementSpec struct {
		metric     types.MetricType
		unit       types.MeasurementUnit
		raw        float64
		normalized *float64
	}

	for _, row := range rows {
		executableDepth := types.ExecutableDepth(row)

		if executableDepth <= 0 {
			continue
		}

		relativeDepth := executableDepth / depthMedian
		scarcity := math.Max(0, 1-relativeDepth)
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
			{types.MetricRelativeTouchDepth, types.UnitDimensionless, relativeDepth, types.NormalizeFinite(relativeDepth)},
			{types.MetricScarcityScore, types.UnitDimensionless, scarcity, types.NormalizeFinite(scarcity)},
			{types.MetricExecutableTouchDepth, types.UnitQuoteCurrency, executableDepth, nil},
			{types.MetricExecutableTouchDepthMedian, types.UnitQuoteCurrency, depthMedian, nil},
		}
		reportedNotional := types.QuoteNotional(row)

		if reportedNotional > 0 && hasNotionalMedian && notionalMedian > 0 {
			specs = append(specs,
				measurementSpec{types.MetricReportedVolumeNotional, types.UnitQuoteCurrency, reportedNotional, nil},
				measurementSpec{types.MetricReportedVolumeNotionalMedian, types.UnitQuoteCurrency, notionalMedian, nil},
			)
		}

		for _, spec := range specs {
			out = append(out, &types.Measurement{
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
			})
		}
	}

	thesis.Signals.Store("liquidity.tickers", rows)
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
