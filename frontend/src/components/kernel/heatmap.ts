import {
	latestByMetric,
	percentOf,
} from "#/components/terminal/measurement-view";
import type { Measurement } from "#/types/measurement";

export type HeatmapCell = {
	symbol: string;
	label: string;
	value: number;
};

type SourceHistory = { values: () => Measurement[] };

/*
heatmapLabel shortens a pair symbol to its base asset for the cell caption.
*/
export const heatmapLabel = (symbol: string): string =>
	symbol.split("/")[0] ?? symbol;

/*
buildHeatmapCells collects the latest headline metric per symbol that has a
reading for the selected source. The grid column count is fixed in the view;
row count follows however many symbols the backend has reported.
*/
export const buildHeatmapCells = (
	measurements: Record<string, Record<string, SourceHistory>>,
	source: string,
	headline: string,
): HeatmapCell[] =>
	Object.entries(measurements).flatMap(([symbol, sources]) => {
		const frame = latestByMetric(sources[source]?.values() ?? [], headline);

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
