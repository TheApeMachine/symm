/*
cross-section projects a backend cross-section frame into the X-ray tile grid.
Each tile is one live symbol row; tiles are derived only from rows the frame
actually carries, so an empty or symbol-less frame yields no tiles rather than
fabricated placeholders.
*/

export type CrossSectionRow = {
	symbol: string;
	vol?: number;
	change_pct?: number;
	re?: number;
};

export type CrossSectionFrame = {
	type?: string;
	symbols?: CrossSectionRow[];
} | null;

export type CrossSectionTile = {
	label: string;
	symbol: string;
	value: number;
	changePercent: number;
	reynolds: number;
};

/*
baseLabel takes the asset base of a pair symbol ("BTC/EUR" → "BTC"). Symbols
without a quote separator pass through unchanged.
*/
const baseLabel = (symbol: string): string => {
	const slash = symbol.indexOf("/");

	return slash > 0 ? symbol.slice(0, slash) : symbol;
};

/*
terminalCrossSectionTiles converts a cross-section frame to tiles. The tile value
is the row's volume (the field magnitude the X-ray heatmap renders); rows without
a positive volume are dropped so a tile always represents real activity. A null,
typed-only, or symbol-less frame produces an empty grid.
*/
export const terminalCrossSectionTiles = (
	frame: CrossSectionFrame,
): CrossSectionTile[] => {
	if (frame === null || frame.symbols === undefined) {
		return [];
	}

	return frame.symbols
		.filter(
			(row) =>
				typeof row.symbol === "string" &&
				row.symbol !== "" &&
				typeof row.vol === "number" &&
				row.vol > 0,
		)
		.map((row) => ({
			label: baseLabel(row.symbol),
			symbol: row.symbol,
			value: row.vol ?? 0,
			changePercent: typeof row.change_pct === "number" ? row.change_pct : 0,
			reynolds: typeof row.re === "number" ? row.re : 0,
		}));
};
