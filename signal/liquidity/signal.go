package liquidity

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
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
	ui     chan []byte
}

func (signal *Signal) Measure(thesis *types.Thesis) chan []*types.Measurement {
	out := make(chan []*types.Measurement)

	go func() {
		defer close(out)

		measurements, err := signal.Calculate(thesis.Market())

		if err != nil {
			errnie.Error(err)
			out <- nil
			return
		}

		out <- measurements
		signal.Publish(measurements)
	}()

	return out
}

/*
NewSignal creates liquidity measurement state for central market cuts so each
tick can compare executable liquidity across the observed cohort.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
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
	filtered := types.FilterLatest(measurements)

	if len(filtered) == 0 {
		return
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": filtered,
	}.Marshal():
	default:
	}
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
	metrics := frame.CrossSection.Metrics
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
		return out, nil
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
