import {
	type measurementsStore,
	measurementTickCount,
} from "#/collections/measurements";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";

/*
sameSources compares ordered source-key lists so React can skip reconciliation
when websocket ticks only update measurement values.
*/
export const sameSources = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((source, index) => source === right[index]);

/*
backendMeasurementSources lists every signal source key currently present in the
measurement store without reading circular-buffer values.
*/
export const backendMeasurementSources = (
	state: typeof measurementsStore.state,
): string[] =>
	orderedKernelSources([
		...new Set(
			Object.values(state.measurements).flatMap((sourceMap) =>
				Object.keys(sourceMap),
			),
		),
	]);

/*
sourceHasUniverseFrames reports whether any symbol currently carries epochs for
the given source, independent of the focused symbol.
*/
export const sourceHasUniverseFrames = (
	state: typeof measurementsStore.state,
	source: string,
): boolean =>
	Object.values(state.measurements).some(
		(sourceMap) => measurementTickCount(sourceMap[source]) > 0,
	);

/*
liveFocusSymbol keeps an explicit preferred focus once that symbol has appeared
in the store. When the preferred symbol has never been observed, it returns the
first live symbol so a silent default focus cannot paint STANDBY forever.
*/
export const liveFocusSymbol = (
	state: typeof measurementsStore.state,
	preferred: string,
): string => {
	if (state.measurements[preferred] !== undefined) {
		return preferred;
	}

	const symbols = Object.keys(state.measurements).sort();

	for (const symbol of symbols) {
		const sourceMap = state.measurements[symbol];

		if (
			sourceMap !== undefined &&
			Object.values(sourceMap).some(
				(buffer) => measurementTickCount(buffer) > 0,
			)
		) {
			return symbol;
		}
	}

	return preferred;
};
