import type { Measurement } from "#/collections/types";

export type HeatmapCell = {
	symbol: string;
	label: string;
	value: number;
};

/*
heatmapLabel shortens a pair symbol to its base asset for the cell caption.
*/
export const heatmapLabel = (symbol: string): string =>
	symbol.split("/")[0] ?? symbol;

/*
buildHeatmapCells collects the latest headline metric per symbol that has a
reading for the selected source from flat measurement history.
*/
export const buildHeatmapCells = (
	measurements: Measurement[],
	source: string,
	headline: string,
): HeatmapCell[] => {
	const bySymbol = new Map<string, Measurement[]>();

	for (const measurement of measurements) {
		if (measurement.source !== source) {
			continue;
		}

		const rows = bySymbol.get(measurement.symbol) ?? [];
		rows.push(measurement);
		bySymbol.set(measurement.symbol, rows);
	}

	return [...bySymbol.entries()]
		.sort(([left], [right]) => left.localeCompare(right))
		.flatMap(([symbol, rows]) => {
			const frame = rows
				.reverse()
				.find((row) => row.metrics?.[headline] !== undefined);

			if (frame === undefined) {
				return [];
			}

			return [
				{
					symbol,
					label: heatmapLabel(symbol),
					value: frame.metrics?.[headline]?.normalized ?? frame.metrics?.[headline]?.raw ?? 0,
				},
			];
		});
};
