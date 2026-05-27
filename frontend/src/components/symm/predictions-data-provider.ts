import type { EnginePulseEvent } from "#/lib/symm/events";
import {
	isEnginePulseEvent,
	isPredictionFeedback,
	type PredictionFeedback,
} from "#/lib/symm/events";
import {
	isSignalSource,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";

export type PredictionReading = {
	source: SignalSource;
	x: number;
	value: number;
};

type ReadingSink = (reading: PredictionReading) => void;

/*
PredictionsDataProvider feeds per-source predicted returns and settled errors.
*/
class PredictionsDataProviderImpl {
	private sink: ReadingSink | null = null;
	private pulse: EnginePulseEvent | undefined;
	private seq = 0;
	private listeners = new Set<() => void>();

	registerSink(sink: ReadingSink) {
		this.sink = sink;

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

	private emit(source: string, value: number) {
		if (!isSignalSource(source) || !Number.isFinite(value)) {
			return;
		}

		this.seq++;
		this.sink?.({ source, x: this.seq, value });
	}

	ingestFeedback(raw: unknown) {
		if (!isPredictionFeedback(raw)) {
			return;
		}

		this.emit(raw.Source, raw.PredictedReturn);
		this.emit(raw.Source, raw.Error);
	}

	ingestPrediction(raw: unknown) {
		if (typeof raw !== "object" || raw === null) {
			return;
		}

		const row = raw as Record<string, unknown>;

		if (row.event !== "prediction") {
			return;
		}

		if (typeof row.source !== "string" || typeof row.value !== "number") {
			return;
		}

		this.emit(row.source, row.value);
	}

	ingestPulse(raw: unknown) {
		if (!isEnginePulseEvent(raw)) {
			return;
		}

		this.pulse = raw;
		this.notify();
	}

	ingest(raw: unknown) {
		if (isEnginePulseEvent(raw)) {
			this.ingestPulse(raw);
			return;
		}

		if (isPredictionFeedback(raw)) {
			this.ingestFeedback(raw);
			return;
		}

		this.ingestPrediction(raw);
	}
}

const shared = new PredictionsDataProviderImpl();

export const PredictionsDataProvider = {
	registerSink: (sink: ReadingSink) => shared.registerSink(sink),
	subscribe: (listener: () => void) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
	sources: SIGNAL_SOURCES,
};

export type { PredictionFeedback };
