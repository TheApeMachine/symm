package liquidity

import (
	"container/ring"
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's quote notional and executable depth against its peers.
Categories belong in logic; this signal emits numerical scores only.
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
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows := make([]kraken.TickerData, 0)
	signal.ticker.cache.Range(func(key, value any) bool {
		value.(*ring.Ring).Do(func(value any) {
			if value != nil {
				rows = append(rows, value.(kraken.TickerData))
			}
		})
		signal.ticker.cache.Delete(key)

		return true
	})
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

	if len(notionalPeers) >= 2 && len(depthPeers) >= 2 {
		notionalMedian, notionalOK := statistic.MedianOf(notionalPeers)
		depthMedian, depthOK := statistic.MedianOf(depthPeers)

		if notionalOK && notionalMedian > 0 && depthOK && depthMedian > 0 {
			peerMaturity := float64(len(notionalPeers)) /
				float64(len(notionalPeers)+1)

			for _, row := range rows {
				notional := types.QuoteNotional(row)
				executableDepth := types.ExecutableDepth(row)

				if notional <= 0 || executableDepth <= 0 {
					continue
				}

				relative := math.Sqrt((notional / notionalMedian) * (executableDepth / depthMedian))
				scarcity := math.Max(0, 1-relative)
				depth := math.Max(0, relative-1)
				balance := 1 / (1 + math.Abs(relative-1))
				strength := max(scarcity, max(balance, depth))
				validity := types.MeasurementValidity{
					State:     types.ValidityValid,
					Readiness: types.ReadinessObservation,
				}
				scale := types.ScaleReference{
					Kind:    types.ScaleObservationWindow,
					From:    row.Timestamp,
					Through: row.Timestamp,
				}
				specs := []struct {
					metric     types.MetricType
					unit       types.MeasurementUnit
					raw        float64
					normalized *float64
				}{
					{types.MetricRVOL, types.UnitDimensionless, relative, types.NormalizeFinite(relative)},
					{types.MetricScarcityScore, types.UnitDimensionless, scarcity, types.NormalizeFinite(scarcity)},
					{types.MetricPeerBalanceScore, types.UnitDimensionless, balance, types.NormalizeFinite(balance)},
					{types.MetricDepthScore, types.UnitDimensionless, depth, types.NormalizeFinite(depth)},
					{types.MetricStrength, types.UnitDimensionless, strength, types.NormalizeFinite(strength)},
					{types.MetricQuoteNotional, types.UnitQuoteCurrency, notional, types.NormalizeRatio(notional, notionalMedian)},
					{types.MetricQuoteNotionalMedian, types.UnitQuoteCurrency, notionalMedian, types.NormalizeFinite(1)},
					{
						types.MetricExecutableDepth,
						types.UnitQuoteCurrency,
						executableDepth,
						types.NormalizeRatio(executableDepth, depthMedian),
					},
					{types.MetricExecutableDepthMedian, types.UnitQuoteCurrency, depthMedian, types.NormalizeFinite(1)},
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
		}
	}

	thesis.Signals.Store("tickers", rows)
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
