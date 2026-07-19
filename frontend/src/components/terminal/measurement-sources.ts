import { measurementTickCount } from "#/collections/measurements";
import type { Measurement } from "#/types/measurement";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";

/*
sameSources compares ordered source-key lists so React can skip reconciliation
when websocket ticks only update measurement values.
*/
export const sameSources = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((source, index) => source === right[index]);

/*
backendMeasurementSources lists every signal source key present in a flat
measurement snapshot without reading circular-buffer stores.
*/
export const backendMeasurementSources = (
	measurements: Measurement[],
): string[] =>
	orderedKernelSources([
		...new Set(measurements.map((measurement) => measurement.source)),
	]);

/*
sourceHasUniverseFrames reports whether any retained row carries the given
source, independent of the focused symbol.
*/
export const sourceHasUniverseFrames = (
	measurements: Measurement[],
	source: string,
): boolean =>
	measurementTickCount(
		measurements.filter((measurement) => measurement.source === source),
	) > 0;

/*
liveFocusSymbol keeps the explicit preferred focus. Auto-picking the first live
symbol made the terminal jump to a lexical random pair at startup; STANDBY on
the preferred major until its frames arrive is the correct cold-start state.
*/
export const liveFocusSymbol = (
	_measurements: Measurement[],
	preferred: string,
): string => preferred;
