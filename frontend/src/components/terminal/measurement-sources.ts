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
liveFocusSymbol keeps the explicit preferred focus. Auto-picking the first live
symbol made the terminal jump to a lexical random pair at startup; STANDBY on
the preferred major until its frames arrive is the correct cold-start state.
*/
export const liveFocusSymbol = (
	_state: typeof measurementsStore.state,
	preferred: string,
): string => preferred;
