export type LayoutPanelType =
	| "prediction_chart"
	| "gauge_grid"
	| "gauge_strip"
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
	anchor_symbol?: string;
	panels: LayoutPanel[];
};

export const defaultLayoutDocument = (): LayoutDocument => ({
	event: "layout",
	ts: new Date(0).toISOString(),
	panels: [
		{ type: "prediction_chart", stream: "prediction" },
		{
			type: "gauge_grid",
			sources: [],
			labels: {},
		},
		{
			type: "gauge_strip",
			sources: [],
			labels: {},
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
	value === "gauge_strip" ||
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
	anchor_symbol: raw.anchor_symbol,
	panels: raw.panels.map((panel) => ({
		...panel,
		sources: panel.sources?.map((source) => source.trim()).filter(Boolean),
	})),
});

export const gaugeSourcesFor = (panel?: LayoutPanel): string[] => {
	if (panel?.sources !== undefined) {
		return [...panel.sources];
	}

	return [];
};

export const hasGaugeSources = (panel?: LayoutPanel): boolean =>
	gaugeSourcesFor(panel).length > 0;

export const gaugeLabelFor = (panel: LayoutPanel, source: string): string =>
	panel.labels?.[source] ?? source;

export const panelsByType = (
	layout: LayoutDocument,
	type: LayoutPanelType,
): LayoutPanel[] => layout.panels.filter((panel) => panel.type === type);

export const GAUGE_GRID_CAPACITY = 8;

export const gaugeGridLayout = (
	sources: readonly string[],
	columns = 4,
): { columns: number; rows: number } => {
	const count = Math.min(sources.length, GAUGE_GRID_CAPACITY);

	if (count === 0) {
		return { columns: 1, rows: 1 };
	}

	const columnCount = Math.min(columns, count);

	return {
		columns: columnCount,
		rows: Math.max(1, Math.ceil(count / columnCount)),
	};
};

export const allGaugeSources = (layout: LayoutDocument): string[] => {
	const gridPanel = panelsByType(layout, "gauge_grid")[0];
	const stripPanel = panelsByType(layout, "gauge_strip")[0];

	return [...gaugeSourcesFor(gridPanel), ...gaugeSourcesFor(stripPanel)];
};

export const mergedGaugePanel = (
	layout: LayoutDocument,
): LayoutPanel | undefined => {
	const gridPanel = panelsByType(layout, "gauge_grid")[0];
	const stripPanel = panelsByType(layout, "gauge_strip")[0];

	if (gridPanel === undefined && stripPanel === undefined) {
		return undefined;
	}

	return {
		type: "gauge_grid",
		sources: allGaugeSources(layout),
		labels: {
			...gridPanel?.labels,
			...stripPanel?.labels,
		},
	};
};
