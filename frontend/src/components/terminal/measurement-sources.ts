import type { measurementsStore } from "#/collections/measurements";
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
