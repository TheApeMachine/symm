package liquidity

import (
	"context"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
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
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

	thesis.CrossSection.Observe(rows)
	view := thesis.CrossSection.Snapshot()
	notionalPeers := view.QuoteNotionals()
	depthPeers := view.ExecutableDepths()

	if len(notionalPeers) >= 2 && len(depthPeers) >= 2 {
		notionalMedian, notionalOK := statistic.MedianOf(notionalPeers)
		depthMedian, depthOK := statistic.MedianOf(depthPeers)

		if notionalOK && notionalMedian > 0 && depthOK && depthMedian > 0 {
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

				out[row.Symbol] = datura.Map[*decimal.Decimal]{
					"rvol":                  decimal.NewFromFloat64(relative),
					"scarcityScore":         decimal.NewFromFloat64(scarcity),
					"medianScore":           decimal.NewFromFloat64(balance),
					"depthScore":            decimal.NewFromFloat64(depth),
					"strength":              decimal.NewFromFloat64(strength),
					"quoteNotional":         decimal.NewFromFloat64(notional),
					"quoteNotionalMedian":   decimal.NewFromFloat64(notionalMedian),
					"executableDepth":       decimal.NewFromFloat64(executableDepth),
					"executableDepthMedian": decimal.NewFromFloat64(depthMedian),
				}
			}
		}
	}

	signal.ticker.cache = signal.ticker.cache[:0]

	thesis.Signals.Store("tickers", rows)
	thesis.Measurements.Store("liquidity", out)

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
