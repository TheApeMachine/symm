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
PredictionsDataProvider feeds per-source prediction series on the grid chart.
Live confidence updates stream continuously; settled feedback appends predicted return.
*/
class PredictionsDataProviderImpl {
	private sink: ReadingSink | null = null;
	private pulse: EnginePulseEvent | undefined;
	private pulseSeq = 0;
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

	private emit(source: SignalSource, value: number) {
		if (!Number.isFinite(value)) {
			return;
		}

		this.sink?.({ source, x: this.pulseSeq, value });
	}

	ingestConfidence(raw: unknown) {
		if (typeof raw !== "object" || raw === null) {
			return;
		}

		const row = raw as Record<string, unknown>;

		if (typeof row.source !== "string" || typeof row.confidence !== "number") {
			return;
		}

		if (!isSignalSource(row.source)) {
			return;
		}

		this.emit(row.source, row.confidence);
	}

	ingestFeedback(raw: unknown) {
		if (!isPredictionFeedback(raw)) {
			return;
		}

		if (!isSignalSource(raw.Source)) {
			return;
		}

		this.emit(raw.Source, raw.PredictedReturn);
	}

	ingestPulse(raw: unknown) {
		if (!isEnginePulseEvent(raw)) {
			return;
		}

		this.pulse = raw;
		this.pulseSeq = raw.seq;
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

		this.ingestConfidence(raw);
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
