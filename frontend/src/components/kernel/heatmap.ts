import type { Measurement } from "#/collections/types";
import {
	latestByMetric,
	percentOf,
} from "#/components/terminal/measurement-view";

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
reading for the selected source from a flat measurement snapshot.
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
			const frame = latestByMetric(rows, headline);

			if (frame === undefined) {
				return [];
			}

			return [
				{
					symbol,
					label: heatmapLabel(symbol),
					value: percentOf(frame) / 100,
				},
			];
		});
};
