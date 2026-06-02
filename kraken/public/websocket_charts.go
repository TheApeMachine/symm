package public

import (
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
)

func (ws *WebSocket) bindChartStreams(streams *focus.Set) {
	if streams == nil {
		return
	}

	ws.streams = streams
	streams.SetStreamNotifier(ws.onStreamChange)
	ws.resubscribeOhlc(focus.AnchorSymbol())

	for _, symbol := range streams.Snapshot() {
		ws.resubscribeOhlc(symbol)
	}
}

func (ws *WebSocket) resubscribeOhlc(symbol string) {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return
	}

	delete(ws.ohlcSubscribed, symbol)
	ws.ensureOhlcSubscription(symbol)
}

func (ws *WebSocket) onStreamChange(symbol string, added bool) {
	if !added {
		return
	}

	ws.ensureOhlcSubscription(symbol)
}

func (ws *WebSocket) ensureOhlcSubscription(symbol string) {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return
	}

	if _, known := ws.ohlcSubscribed[symbol]; known {
		return
	}

	outbound := ws.broadcasts["kraken:public"]

	if outbound == nil {
		return
	}

	ws.ohlcSubscribed[symbol] = struct{}{}

	outbound.Send(&qpool.QValue[any]{
		Value: OhlcSubscribeFrame(symbol),
	})
}

// applyOhlc publishes Kraken v2 ohlc rows as candle_bar frames for chart-stream
// symbols only. Updates share interval_begin so the frontend updates bars in place.
func (ws *WebSocket) applyOhlc(msg SocketMessage) error {
	charts := ws.broadcasts["ui:charts"]

	if charts == nil {
		return fmt.Errorf("kraken public websocket: ui:charts broadcast missing")
	}

	candles, err := DecodeOhlc(&msg)

	if err != nil {
		return fmt.Errorf("kraken public websocket: decode ohlc: %w", err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	for _, candle := range candles {
		if candle.Symbol == "" {
			continue
		}

		if ws.streams != nil && !ws.streams.Streams(candle.Symbol) {
			continue
		}

		sec, secErr := CandleIntervalSec(candle)

		if secErr != nil {
			return fmt.Errorf("kraken public websocket: ohlc interval %s: %w", candle.Symbol, secErr)
		}

		charts.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":          "candle_bar",
			"ts":             nowStr,
			"symbol":         candle.Symbol,
			"sec":            sec,
			"interval_begin": candle.IntervalBegin,
			"interval":       candle.Interval,
			"open":           candle.Open,
			"high":           candle.High,
			"low":            candle.Low,
			"close":          candle.Close,
			"volume":         candle.Volume,
			"trades":         candle.Trades,
			"vwap":           candle.VWAP,
		}})
	}

	return nil
}
