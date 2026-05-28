import type { EnginePulseEvent } from "#/lib/symm/events";
import {
	isEnginePulseEvent,
	isPredictionEvent,
	isPredictionSettledEvent,
} from "#/lib/symm/events";

export type PredictionSeriesKind =
	| "average"
	| "prediction"
	| "error"
	| "actual";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	x: number;
	value: number;
};

type ReadingSink = (reading: PredictionReading) => void;

const MAX_BUFFER = 1200;

const timeSec = (value: unknown): number | undefined => {
	if (typeof value !== "string" || value.length === 0) {
		return undefined;
	}

	const parsed = Date.parse(value);

	if (!Number.isFinite(parsed)) {
		return undefined;
	}

	return parsed / 1000;
};

class PredictionsDataProviderImpl {
	private sink: ReadingSink | null = null;
	private pulse: EnginePulseEvent | undefined;
	private buffer: PredictionReading[] = [];
	private listeners = new Set<() => void>();
	private previousPulseSec: number | undefined;
	private pulseHorizonSec: number | undefined;

	registerSink(sink: ReadingSink) {
		this.sink = sink;

		for (const reading of this.buffer) {
			sink(reading);
		}

		return () => {
			if (this.sink === sink) {
				this.sink = null;
			}
		};
	}

	subscribe(listener: () => void) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): EnginePulseEvent | undefined {
		return this.pulse;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	private push(reading: PredictionReading) {
		this.buffer.push(reading);

		if (this.buffer.length > MAX_BUFFER) {
			this.buffer = this.buffer.slice(-MAX_BUFFER);
		}

		this.sink?.(reading);
	}

	private emitPoint(
		kind: PredictionSeriesKind,
		x: number,
		value: number,
	) {
		if (!Number.isFinite(value) || !Number.isFinite(x)) {
			return;
		}

		this.push({
			kind,
			x,
			value,
		});
	}

	private updateHorizon(pulseSec: number) {
		if (this.previousPulseSec === undefined) {
			this.previousPulseSec = pulseSec;
			return;
		}

		const observedHorizonSec = pulseSec - this.previousPulseSec;
		this.previousPulseSec = pulseSec;

		if (!Number.isFinite(observedHorizonSec) || observedHorizonSec <= 0) {
			return;
		}

		this.pulseHorizonSec = observedHorizonSec;
	}

	ingestPulse(raw: unknown) {
		if (!isEnginePulseEvent(raw)) {
			return;
		}

		this.pulse = raw;
		this.notify();

		const pulseSec = timeSec(raw.ts);

		if (pulseSec === undefined) {
			return;
		}

		if (typeof raw.avg_prediction === "number") {
			this.emitPoint("average", pulseSec, raw.avg_prediction);
		}

		this.updateHorizon(pulseSec);
	}

	ingest(raw: unknown) {
		if (isEnginePulseEvent(raw)) {
			this.ingestPulse(raw);
			return;
		}

		if (isPredictionEvent(raw)) {
			// Plot the forecast at its anchored emission time. Each prediction
			// has its own clock — anchored at `ts`, due at `due_at` — so the
			// chart shows the forecast where it lives in time, not at a
			// per-cycle index.
			const tsSec = timeSec(raw.ts);
			if (tsSec !== undefined) {
				this.emitPoint("prediction", tsSec, raw.value);
			}
			return;
		}

		if (isPredictionSettledEvent(raw)) {
			// Place the realised return and error at the settlement instant so
			// the chart can compare predicted vs actual at the same x-axis
			// position the forecast pointed to.
			const settledSec = timeSec(raw.ts);
			if (settledSec === undefined) {
				return;
			}
			this.emitPoint("actual", settledSec, raw.actual_return);
			this.emitPoint("error", settledSec, raw.error);
			return;
		}
	}

	reset() {
		this.sink = null;
		this.pulse = undefined;
		this.buffer = [];
		this.previousPulseSec = undefined;
		this.pulseHorizonSec = undefined;
		this.notify();
	}
}

const shared = new PredictionsDataProviderImpl();

export const PredictionsDataProvider = {
	registerSink: (sink: ReadingSink) => shared.registerSink(sink),
	subscribe: (listener: () => void) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
	reset: () => shared.reset(),
};
