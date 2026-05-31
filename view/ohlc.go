package view

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
)

// reconcileInterval is how often the feed compares its running candle streams to
// the open-position set.
const reconcileInterval = time.Second

// candleIntervalMinutes is the OHLC bar size streamed to the chart.
const candleIntervalMinutes = 1

/*
OHLC streams candle bars to the dashboard for symbols with an open position only.
The trade desk owns the focus set; this feed reconciles against it, opening a
candle subscription when a position opens and closing it when the position is
gone, so the "ui" bus never carries chart data for symbols we are not trading.
*/
type OHLC struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pool    *qpool.Q
	ui      *qpool.BroadcastGroup
	tracker *focus.Set
	mu      sync.Mutex
	streams map[string]context.CancelFunc
}

/*
NewOHLC builds the OHLC dashboard feed bound to the shared focus set.
*/
func NewOHLC(ctx context.Context, pool *qpool.Q, tracker *focus.Set) *OHLC {
	ctx, cancel := context.WithCancel(ctx)

	return &OHLC{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		ui:      pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		tracker: tracker,
		streams: make(map[string]context.CancelFunc),
	}
}

/*
Tick reconciles running streams against the open-position set on a fixed cadence.
*/
func (ohlc *OHLC) Tick() error {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	ohlc.reconcile()

	for {
		select {
		case <-ohlc.ctx.Done():
			return ohlc.ctx.Err()
		case <-ticker.C:
			ohlc.reconcile()
		}
	}
}

// reconcile starts a candle stream for every newly opened position and stops the
// stream for any symbol whose position has closed.
func (ohlc *OHLC) reconcile() {
	wanted := map[string]struct{}{focus.AnchorSymbol: {}}

	for _, symbol := range ohlc.tracker.Snapshot() {
		wanted[symbol] = struct{}{}
	}

	for symbol := range wanted {
		ohlc.mu.Lock()
		_, running := ohlc.streams[symbol]
		ohlc.mu.Unlock()

		if !running {
			ohlc.start(symbol)
		}
	}

	ohlc.mu.Lock()
	defer ohlc.mu.Unlock()

	for symbol, cancel := range ohlc.streams {
		if _, keep := wanted[symbol]; keep {
			continue
		}

		cancel()
		delete(ohlc.streams, symbol)
	}
}

// start opens a candle subscription for symbol and ships each bar to the ui bus.
func (ohlc *OHLC) start(symbol string) {
	streamCtx, cancel := context.WithCancel(ohlc.ctx)

	ohlc.mu.Lock()
	ohlc.streams[symbol] = cancel
	ohlc.mu.Unlock()

	go func() {
		for bar := range market.NewCandleSubscription(
			streamCtx, candleIntervalMinutes, symbol,
		) {
			ohlc.ui.Send(&qpool.QValue[any]{Value: ohlc.frame(symbol, bar)})
		}
	}()
}

// frame shapes one candle bar into the candle_bar wire frame the chart consumes.
func (ohlc *OHLC) frame(symbol string, bar *market.CandleUpdate) map[string]any {
	return map[string]any{
		"event":  "candle_bar",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"symbol": symbol,
		"sec":    barSeconds(bar.IntervalBegin),
		"open":   bar.Open,
		"high":   bar.High,
		"low":    bar.Low,
		"close":  bar.Close,
		"volume": bar.Volume,
	}
}

func (ohlc *OHLC) Close() error {
	ohlc.cancel()
	return nil
}

// barSeconds converts the candle's interval-begin timestamp to unix seconds,
// falling back to now when it is missing or unparseable.
func barSeconds(intervalBegin string) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, intervalBegin)

	if err != nil || parsed.IsZero() {
		return time.Now().Unix()
	}

	return parsed.Unix()
}
