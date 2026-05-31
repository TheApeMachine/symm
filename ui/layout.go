package ui

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
LayoutPanel describes one dashboard visualization bound to backend telemetry streams.
*/
type LayoutPanel struct {
	Type        string            `json:"type"`
	Stream      string            `json:"stream,omitempty"`
	Sources     []string          `json:"sources,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	HeightKey   string            `json:"height_key,omitempty"`
	SymbolsFrom string            `json:"symbols_from,omitempty"`
}

/*
LayoutDocument is the schema-driven dashboard layout shipped on websocket connect.
*/
type LayoutDocument struct {
	Event  string        `json:"event"`
	TS     string        `json:"ts"`
	Panels []LayoutPanel `json:"panels"`
}

/*
DefaultDashboardLayout returns the canonical telemetry layout for the trading desk.
*/
func DefaultDashboardLayout(now time.Time) LayoutDocument {
	sources := []string{
		perspectives.SourceHawkes.String(),
		perspectives.SourceFluid.String(),
		perspectives.SourcePumpDump.String(),
		perspectives.SourceCausal.String(),
		perspectives.SourceDepthFlow.String(),
		perspectives.SourceLeadLag.String(),
		perspectives.SourceLiquidity.String(),
		perspectives.SourceSentiment.String(),
	}

	return LayoutDocument{
		Event: "layout",
		TS:    now.UTC().Format(time.RFC3339Nano),
		Panels: []LayoutPanel{
			{
				Type:   "prediction_chart",
				Stream: "prediction",
			},
			{
				Type:    "gauge_grid",
				Sources: sources,
				Labels:  defaultGaugeLabels(sources),
			},
			{
				Type:        "trade_grid",
				SymbolsFrom: "wallet.inventory",
				Stream:      "candle_bar",
			},
			{
				Type:   "trades_panel",
				Stream: "wallet",
			},
			{
				Type:   "audit_panel",
				Stream: "audit",
			},
			{
				Type:      "surface",
				Stream:    "field_snapshot",
				HeightKey: "grid.heights",
			},
		},
	}
}

func defaultGaugeLabels(sources []string) map[string]string {
	labels := map[string]string{
		"hawkes":    "Hawkes",
		"fluid":     "Fluid",
		"pumpdump":  "Pump",
		"causal":    "Causal",
		"depthflow": "Depth",
		"leadlag":   "LeadLag",
		"liquidity": "Basis",
		"sentiment": "Sent",
	}

	for _, source := range sources {
		if _, ok := labels[source]; ok {
			continue
		}

		labels[source] = source
	}

	return labels
}

func (document LayoutDocument) Wire() map[string]any {
	panels := make([]map[string]any, 0, len(document.Panels))

	for _, panel := range document.Panels {
		wire := map[string]any{
			"type": panel.Type,
		}

		if panel.Stream != "" {
			wire["stream"] = panel.Stream
		}

		if len(panel.Sources) > 0 {
			wire["sources"] = panel.Sources
		}

		if len(panel.Labels) > 0 {
			wire["labels"] = panel.Labels
		}

		if panel.HeightKey != "" {
			wire["height_key"] = panel.HeightKey
		}

		if panel.SymbolsFrom != "" {
			wire["symbols_from"] = panel.SymbolsFrom
		}

		panels = append(panels, wire)
	}

	return map[string]any{
		"event":  document.Event,
		"ts":     document.TS,
		"panels": panels,
	}
}
