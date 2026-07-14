package liquidity

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
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

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(ctx, api),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows := signal.ticker.cache
	out := make([]*types.Measurement, 0, len(rows))

	thesis.CrossSection.ProcessUpdates(rows)

	metrics := thesis.CrossSection.ReadView().Metrics
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

				out = append(out,
					types.ObservationMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricRVOL,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitDimensionless, relative, peerMaturity,
					),
					types.ObservationMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricScarcityScore,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitDimensionless, scarcity, peerMaturity,
					),
					types.ObservationMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricPeerBalanceScore,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitDimensionless, balance, peerMaturity,
					),
					types.ObservationMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricDepthScore,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitDimensionless, depth, peerMaturity,
					),
					types.ObservationMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricStrength,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitDimensionless, strength, peerMaturity,
					),
					types.ObservationNormalizedMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricQuoteNotional,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitQuoteCurrency, notional, peerMaturity,
						types.NormalizeRatio(notional, notionalMedian),
					),
					types.ObservationNormalizedMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricQuoteNotionalMedian,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitQuoteCurrency, notionalMedian, peerMaturity,
						types.NormalizeFinite(1),
					),
					types.ObservationNormalizedMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricExecutableDepth,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitQuoteCurrency, executableDepth, peerMaturity,
						types.NormalizeRatio(executableDepth, depthMedian),
					),
					types.ObservationNormalizedMeasurement(
						types.SourceLiquidity, types.Liquidity, types.MetricExecutableDepthMedian,
						types.SubjectPeerLiquidity, row.Symbol, row.Timestamp,
						types.UnitQuoteCurrency, depthMedian, peerMaturity,
						types.NormalizeFinite(1),
					),
				)
			}
		}
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
