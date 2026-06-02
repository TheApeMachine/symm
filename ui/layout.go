package ui

import (
	"time"

	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
LayoutDocument builds the dashboard schema the frontend parses on connect.
*/
func LayoutDocument() map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sources := perspectives.DashboardGaugeNames()

	return map[string]any{
		"event":         "layout",
		"ts":            now,
		"anchor_symbol": focus.AnchorSymbol(),
		"panels": []map[string]any{
			{
				"type":   "prediction_chart",
				"stream": "prediction",
			},
			{
				"type":    "gauge_grid",
				"sources": sources,
				"labels":  perspectives.DashboardGaugeLabelMap(),
			},
			{
				"type":         "trade_grid",
				"stream":       "candle_bar",
				"symbols_from": "wallet.inventory",
			},
			{
				"type":   "trades_panel",
				"stream": "wallet",
			},
			{
				"type":   "audit_panel",
				"stream": "audit",
			},
			{
				"type":       "surface",
				"stream":     "field_snapshot",
				"height_key": "grid.heights",
			},
		},
	}
}
