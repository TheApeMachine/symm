import type { EnginePulseEvent } from "#/lib/symm/events";
import { isEnginePulseEvent } from "#/lib/symm/events";

export type PredictionSeriesKind =
	| "average"
	| "prediction"
	| "error";

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

const finiteNumber = (value: unknown): number | undefined => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return undefined;
	}

	return value;
};

const scaledValue = (
	preferred: unknown,
	fallback: unknown,
): number | undefined => finiteNumber(preferred) ?? finiteNumber(fallback);

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

	private updateHorizon(pulseSec: number): number | undefined {
		if (this.previousPulseSec === undefined) {
			this.previousPulseSec = pulseSec;
			return undefined;
		}

		const observedHorizonSec = pulseSec - this.previousPulseSec;
		this.previousPulseSec = pulseSec;

		if (!Number.isFinite(observedHorizonSec) || observedHorizonSec <= 0) {
			return this.pulseHorizonSec;
		}

		this.pulseHorizonSec = observedHorizonSec;

		return observedHorizonSec;
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

		const horizonSec = this.updateHorizon(pulseSec);

		const predictionValue = scaledValue(
			raw.avg_prediction_multiple,
			raw.avg_prediction,
		);
		const errorValue = scaledValue(raw.avg_error_multiple, raw.avg_error);

		if (predictionValue !== undefined) {
			this.emitPoint("average", pulseSec, predictionValue);

			if (horizonSec !== undefined) {
				this.emitPoint("prediction", pulseSec + horizonSec, predictionValue);
			}
		}

		if (errorValue !== undefined) {
			this.emitPoint("error", pulseSec, errorValue);
		}
	}

	ingest(raw: unknown) {
		if (isEnginePulseEvent(raw)) {
			this.ingestPulse(raw);
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
