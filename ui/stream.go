package ui

import (
	"time"

	"github.com/theapemachine/symm/fluid"
)

/*
MarketStream pushes telemetry the moment market or engine data is available.
*/
type MarketStream struct {
	hub     *Hub
	candles *CandleAggregator
}

/*
NewMarketStream binds a hub for non-blocking event fan-out.
*/
func NewMarketStream(hub *Hub) *MarketStream {
	if hub == nil {
		return nil
	}

	return &MarketStream{
		hub:     hub,
		candles: NewCandleAggregator(),
	}
}

/*
Emit forwards a flat event to websocket clients without blocking producers.
*/
func (stream *MarketStream) Emit(event map[string]any) {
	if stream == nil || stream.hub == nil {
		return
	}

	stream.hub.Emit(omitEmptyCollections(event))
}

/*
PriceTick publishes one live quote update.
*/
func (stream *MarketStream) PriceTick(
	symbol string,
	last, bid, ask, changePct float64,
	at string,
) {
	if stream == nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	if at == "" {
		at = now
	}

	stream.Emit(map[string]any{
		"event":          "price_tick",
		"ts":             now,
		"symbol":         symbol,
		"last":           last,
		"bid":            bid,
		"ask":            ask,
		"change_pct_24h": changePct,
		"at":             at,
	})

	if stream.candles == nil {
		return
	}

	parsedAt, err := time.Parse(time.RFC3339Nano, at)

	if err != nil || parsedAt.IsZero() {
		parsedAt = time.Now().UTC()
	}

	if bar, ok := stream.candles.Update(symbol, last, parsedAt); ok {
		stream.Emit(map[string]any{
			"event":  "candle_bar",
			"ts":     now,
			"symbol": symbol,
			"sec":    bar.Sec,
			"open":   bar.Open,
			"high":   bar.High,
			"low":    bar.Low,
			"close":  bar.Close,
		})
	}
}

/*
FieldUpdate publishes the latest fluid field rows as soon as they exist.
*/
func (stream *MarketStream) FieldUpdate(snapshot fluid.FieldSnapshot) {
	if stream == nil {
		return
	}

	rows := make([]map[string]any, 0, len(snapshot.Symbols))

	for _, row := range snapshot.Symbols {
		rows = append(rows, map[string]any{
			"symbol":     row.Symbol,
			"change_pct": row.ChangePct,
			"vol":        row.Vol,
			"div":        row.Div,
			"vort":       row.Vort,
			"turb":       row.Turb,
			"visc":       row.Visc,
			"re":         row.Re,
		})
	}

	stream.Emit(map[string]any{
		"event":        "field_snapshot",
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"symbol_count": snapshot.SymbolCount,
		"field": map[string]any{
			"div":  snapshot.Field.Div,
			"vort": snapshot.Field.Vort,
			"turb": snapshot.Field.Turb,
			"visc": snapshot.Field.Visc,
			"re":   snapshot.Field.Re,
		},
		"symbols": rows,
		"grid": map[string]any{
			"size":         snapshot.Grid.Size,
			"heights":      snapshot.Grid.Heights,
			"min":          snapshot.Grid.Min,
			"max":          snapshot.Grid.Max,
			"filled_cells": snapshot.Grid.FilledCells,
			"outliers": map[string]any{
				"clipped_count":  snapshot.Grid.Outliers.ClippedCount,
				"clipped_at":     snapshot.Grid.Outliers.ClippedAt,
				"raw_max":        snapshot.Grid.Outliers.RawMax,
				"raw_max_symbol": snapshot.Grid.Outliers.RawMaxSymbol,
				"display_max":    snapshot.Grid.Outliers.DisplayMax,
			},
		},
	})
}

/*
EnginePulse publishes one engine heartbeat with live counters and signal rows.
*/
func (stream *MarketStream) EnginePulse(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "engine_pulse"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}

/*
Scoreboard publishes ranked scan targets for the dashboard.
*/
func (stream *MarketStream) Scoreboard(
	line, median, mad float64,
	targets []map[string]any,
) {
	if stream == nil {
		return
	}

	stream.Emit(map[string]any{
		"event":   "scoreboard",
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"line":    line,
		"median":  median,
		"mad":     mad,
		"targets": targets,
	})
}
