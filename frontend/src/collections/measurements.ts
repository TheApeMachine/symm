import { createStore } from "@tanstack/react-store";

export const MEASUREMENT_HISTORY_LIMIT = 96;

export type MeasurementHistorySample = {
	stamp: string;
	observed_at?: number;
	confidence?: number;
	surprise?: number;
	strength?: number;
	category?: string;
};

const numberValue = (value: unknown): number | undefined =>
	typeof value === "number" && Number.isFinite(value) ? value : undefined;

const stringValue = (value: unknown): string | undefined =>
	typeof value === "string" && value.trim() !== "" ? value : undefined;

const outputOf = (frame: Record<string, unknown>): Record<string, unknown> =>
	(frame.output ?? {}) as Record<string, unknown>;

const frameStamp = (frame: Record<string, unknown>): string => {
	const output = outputOf(frame);
	const stamp =
		frame.timestamp_unix_nano ??
		frame.observed_at ??
		frame.timestamp ??
		frame.ts ??
		output.timestamp ??
		output.ts;

	return stamp === undefined ? "" : String(stamp);
};

const frameSample = (
	frame: Record<string, unknown>,
): MeasurementHistorySample => {
	const output = outputOf(frame);
	const confidence =
		numberValue(frame.confidence) ?? numberValue(output.confidence);
	const surprise = numberValue(frame.surprise) ?? numberValue(output.surprise);
	const strength = numberValue(frame.strength) ?? numberValue(output.strength);
	const category = stringValue(frame.category) ?? stringValue(output.category);
	const observedAt =
		numberValue(frame.observed_at) ?? numberValue(output.observed_at);
	const sample: MeasurementHistorySample = {
		stamp: frameStamp(frame),
	};

	if (observedAt !== undefined) sample.observed_at = observedAt;
	if (confidence !== undefined) sample.confidence = confidence;
	if (surprise !== undefined) sample.surprise = surprise;
	if (strength !== undefined) sample.strength = strength;
	if (category !== undefined) sample.category = category;

	return sample;
};

const historyOf = (
	frame: Record<string, unknown> | undefined,
): MeasurementHistorySample[] =>
	Array.isArray(frame?.history)
		? (frame.history as MeasurementHistorySample[])
		: [];

const withoutHistory = (
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const { history: _history, ...rest } = frame;

	return rest;
};

const withHistory = (
	previous: Record<string, unknown> | undefined,
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const sample = frameSample(frame);
	const prior = historyOf(previous);
	const last = prior.at(-1);
	const nextHistory =
		last?.stamp !== "" && last?.stamp === sample.stamp
			? [...prior.slice(0, -1), sample]
			: [...prior, sample];

	return {
		...withoutHistory(frame),
		history: nextHistory.slice(-MEASUREMENT_HISTORY_LIMIT),
	};
};

export const measurementsStore = createStore(
	{} as Record<string, Record<string, Record<string, unknown>>>,
	({ setState }) => ({
		updateReading: (frame: Record<string, unknown>) =>
			measurementsStore.actions.updateReadings([frame]),
		updateReadings: (frames: Record<string, unknown>[]) => {
			if (frames.length === 0) {
				return;
			}

			setState((prev) => {
				const next = { ...prev };
				const touched = new Set<string>();

				for (const frame of frames) {
					const origin = typeof frame.origin === "string" ? frame.origin : "";
					const scope = typeof frame.scope === "string" ? frame.scope : "";

					if (origin === "" || scope === "") {
						continue;
					}

					const byScope =
						next[origin] === undefined || touched.has(origin)
							? (next[origin] ?? {})
							: { ...next[origin] };
					touched.add(origin);
					byScope[scope] = withHistory(byScope[scope], frame);
					next[origin] = byScope;
				}

				return touched.size === 0 ? prev : next;
			});
		},
		reset: () => {
			setState(() => ({}));
		},
	}),
);
