import type { EnginePulseEvent } from "#/lib/symm/events";
import { isEnginePulseEvent } from "#/lib/symm/events";

export type PredictionSeriesKind = "average" | "prediction" | "error";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	x: number;
	value: number;
};

type ReadingSink = (reading: PredictionReading) => void;
type PulseListener = () => void;

const GAUGE_FULL_SIGMA = 4;
const MAX_PLOT_MULTIPLE = 8;

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

const plotMultiple = (
	preferred: unknown,
	snrFallback: unknown,
): number | undefined => {
	const multiple = finiteNumber(preferred);

	if (multiple !== undefined) {
		return Math.min(MAX_PLOT_MULTIPLE, Math.max(0, multiple));
	}

	const snr = finiteNumber(snrFallback);

	if (snr === undefined) {
		return undefined;
	}

	return Math.min(MAX_PLOT_MULTIPLE, Math.max(0, snr / GAUGE_FULL_SIGMA));
};

/*
PredictionsChartAdapter turns engine_pulse wire frames into chart readings.
Series state lives in SciChart; this only parses and forwards appendReading calls.
*/
class PredictionsChartAdapter {
	private sink: ReadingSink | null = null;
	private pulse: EnginePulseEvent | undefined;
	private listeners = new Set<PulseListener>();
	private previousPulseSec: number | undefined;
	private priorPulseMultiple: number | undefined;

	registerSink(sink: ReadingSink) {
		this.sink = sink;

		return () => {
			if (this.sink === sink) {
				this.sink = null;
			}
		};
	}

	subscribe(listener: PulseListener) {
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

	private emitPoint(kind: PredictionSeriesKind, x: number, value: number) {
		if (!Number.isFinite(value) || !Number.isFinite(x)) {
			return;
		}

		this.sink?.({
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
			return undefined;
		}

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

		const realizedMultiple = plotMultiple(
			raw.avg_prediction_multiple,
			raw.avg_prediction,
		);
		const wireError = plotMultiple(raw.avg_error_multiple, raw.avg_error);

		if (realizedMultiple === undefined) {
			return;
		}

		this.emitPoint("average", pulseSec, realizedMultiple);

		if (this.priorPulseMultiple !== undefined) {
			const catchUpError = Math.abs(realizedMultiple - this.priorPulseMultiple);
			const errorValue = wireError ?? catchUpError;

			this.emitPoint("error", pulseSec, errorValue);
		} else if (wireError !== undefined) {
			this.emitPoint("error", pulseSec, wireError);
		}

		if (horizonSec !== undefined) {
			this.emitPoint("prediction", pulseSec + horizonSec, realizedMultiple);
		}

		this.priorPulseMultiple = realizedMultiple;
	}

	ingest(raw: unknown) {
		if (isEnginePulseEvent(raw)) {
			this.ingestPulse(raw);
		}
	}

	reset() {
		this.sink = null;
		this.pulse = undefined;
		this.previousPulseSec = undefined;
		this.priorPulseMultiple = undefined;
		this.notify();
	}
}

const shared = createPredictionsChartAdapter();

export const createPredictionsDataProvider = () =>
	createPredictionsChartAdapter();

function createPredictionsChartAdapter() {
	const adapter = new PredictionsChartAdapter();

	return {
		registerSink: (sink: ReadingSink) => adapter.registerSink(sink),
		subscribe: (listener: PulseListener) => adapter.subscribe(listener),
		snapshot: () => adapter.snapshot(),
		ingest: (raw: unknown) => adapter.ingest(raw),
		reset: () => adapter.reset(),
	};
}

export type PredictionsStore = ReturnType<typeof createPredictionsDataProvider>;

export const PredictionsDataProvider: PredictionsStore = shared;
