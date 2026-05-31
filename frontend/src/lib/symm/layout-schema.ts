export type LayoutPanelType =
	| "prediction_chart"
	| "gauge_grid"
	| "trade_grid"
	| "trades_panel"
	| "audit_panel"
	| "surface";

export type LayoutPanel = {
	type: LayoutPanelType;
	stream?: string;
	sources?: string[];
	labels?: Record<string, string>;
	height_key?: string;
	symbols_from?: string;
};

export type LayoutDocument = {
	event: "layout";
	ts: string;
	panels: LayoutPanel[];
};

const DEFAULT_GAUGE_SOURCES = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"liquidity",
	"sentiment",
] as const;

const DEFAULT_GAUGE_LABELS: Record<string, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
	depthflow: "Depth",
	leadlag: "LeadLag",
	liquidity: "Basis",
	sentiment: "Sent",
};

export const defaultLayoutDocument = (): LayoutDocument => ({
	event: "layout",
	ts: new Date(0).toISOString(),
	panels: [
		{ type: "prediction_chart", stream: "prediction" },
		{
			type: "gauge_grid",
			sources: [...DEFAULT_GAUGE_SOURCES],
			labels: { ...DEFAULT_GAUGE_LABELS },
		},
		{
			type: "trade_grid",
			stream: "candle_bar",
			symbols_from: "wallet.inventory",
		},
		{ type: "trades_panel", stream: "wallet" },
		{ type: "audit_panel", stream: "audit" },
		{
			type: "surface",
			stream: "field_snapshot",
			height_key: "grid.heights",
		},
	],
});

const isLayoutPanelType = (value: string): value is LayoutPanelType =>
	value === "prediction_chart" ||
	value === "gauge_grid" ||
	value === "trade_grid" ||
	value === "trades_panel" ||
	value === "audit_panel" ||
	value === "surface";

export const isLayoutDocument = (raw: unknown): raw is LayoutDocument => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	if (row.event !== "layout" || !Array.isArray(row.panels)) {
		return false;
	}

	return row.panels.every((panel) => {
		if (typeof panel !== "object" || panel === null) {
			return false;
		}

		const typed = panel as Record<string, unknown>;

		return typeof typed.type === "string" && isLayoutPanelType(typed.type);
	});
};

export const normalizeLayoutDocument = (
	raw: LayoutDocument,
): LayoutDocument => ({
	event: "layout",
	ts: raw.ts,
	panels: raw.panels.map((panel) => ({
		...panel,
		sources: panel.sources?.map((source) => source.trim()).filter(Boolean),
	})),
});

export const gaugeLabelFor = (panel: LayoutPanel, source: string): string =>
	panel.labels?.[source] ?? source;

export const panelsByType = (
	layout: LayoutDocument,
	type: LayoutPanelType,
): LayoutPanel[] => layout.panels.filter((panel) => panel.type === type);
