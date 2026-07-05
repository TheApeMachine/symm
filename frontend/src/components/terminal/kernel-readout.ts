import type { SignalHealthStatus } from "#/components/terminal/kernel-meta";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";
import { isConcreteSymbol } from "./scoped-frame";

const BACKEND_STATUSES = new Set<string>([
  "waiting",
  "standby",
  "calibrating",
  "fault",
  "ambiguous",
  "measured",
  "unknown",
]);

const finiteScore = (value: unknown): number => {
  const number = typeof value === "number" ? value : Number(value);

  return Number.isFinite(number) ? Math.min(1, Math.max(0, number)) : 0;
};

const numberFrom = (value: unknown): number => {
  const number = typeof value === "number" ? value : Number(value);

  return Number.isFinite(number) ? number : 0;
};

export type ReadingsState = {
  measurements: Record<string, { values: () => Record<string, unknown>[] }>;
  symbols: Record<string, Record<string, unknown>[]>;
};

export type KernelSparkHistory = {
  scope: string;
  stamp: string;
  values: number[];
};

export const kernelReadingSource = (source: string): string =>
  source === "prediction" ? "resonance" : source;

export const kernelFrameForSource = (
  readings: ReadingsState,
  source: string,
  focusSymbol: string,
): Record<string, unknown> | undefined => {
  const sourceName = kernelReadingSource(source);

  if (isConcreteSymbol(focusSymbol)) {
    for (
      let index = (readings.symbols[focusSymbol] ?? []).length - 1;
      index >= 0;
      index -= 1
    ) {
      const frame = readings.symbols[focusSymbol]?.[index];

      if (frame?.source === sourceName) {
        return frame;
      }
    }

    return undefined;
  }

  return readings.measurements[sourceName]?.values().at(-1);
};

export const kernelStatus = (
  frame: Record<string, unknown> | undefined,
): SignalHealthStatus => {
  if (frame === undefined) {
    return "waiting";
  }

  const status = typeof frame.status === "string" ? frame.status : "";

  if (BACKEND_STATUSES.has(status)) {
    return status as SignalHealthStatus;
  }

  const confidence = finiteScore(frame.confidence);
  const strength = finiteScore(frame.strength);

  if (confidence > 0 || strength > 0) {
    return "measured";
  }

  return "unknown";
};

export const kernelReadout = (frame: Record<string, unknown> | undefined) => {
  const output = frame ?? {};
  const stamp =
    frame?.at ??
    frame?.observed_at ??
    frame?.timestamp_unix_nano ??
    frame?.timestamp ??
    frame?.ts;

  return {
    output,
    confidence: finiteScore(output.confidence),
    surprise: Math.max(0, numberFrom(output.surprise)),
    strength: Math.max(0, numberFrom(output.strength)),
    status: kernelStatus(frame),
    stamp: stamp === undefined ? "" : String(stamp),
  };
};

export const kernelHistoryValues = (
	frame: Record<string, unknown> | undefined,
): number[] =>
	(Array.isArray(frame?.history) ? frame.history : []).flatMap((sample) => {
		const output = sample as Record<string, unknown>;

		return output.confidence === undefined && output.strength === undefined
			? []
			: [finiteScore(output.confidence ?? output.strength)];
	});

export const kernelCollectionValues = (
	readings: ReadingsState,
	source: string,
	focusSymbol: string,
): number[] => {
	const sourceName = kernelReadingSource(source);
	const frames = isConcreteSymbol(focusSymbol)
		? (readings.symbols[focusSymbol] ?? []).filter(
				(frame) => frame.source === sourceName,
			)
		: (readings.measurements[sourceName]?.values() ?? []);

	return frames.flatMap((frame) => {
		const score = frame.confidence ?? frame.strength;

		return score === undefined ? [] : [finiteScore(score)];
	});
};

export const kernelHistoryCount = (
  frame: Record<string, unknown> | undefined,
): number => (Array.isArray(frame?.history) ? frame.history.length : 0);

export const appendKernelSparkHistory = (
  history: KernelSparkHistory,
  scope: string,
  stamp: string,
  sample: unknown,
  limit = 40,
): KernelSparkHistory => {
  if (history.scope === scope && history.stamp === stamp) {
    return history;
  }

  const values = [
    ...(history.scope === scope ? history.values : []),
    finiteScore(sample),
  ].slice(-Math.max(1, limit));

  return { scope, stamp, values };
};

export const kernelHealthSummary = (
  readings: ReadingsState,
  focusSymbol: string,
  origins?: string[],
) => {
  const sources = orderedKernelSources(
    origins ?? Object.keys(readings.measurements),
  );
  let measured = 0;

  for (const origin of sources) {
    if (
      kernelReadout(kernelFrameForSource(readings, origin, focusSymbol))
        .status === "measured"
    ) {
      measured += 1;
    }
  }

  return {
    measured,
    total: sources.length,
    label: `${measured}/${sources.length} measured`,
  };
};
