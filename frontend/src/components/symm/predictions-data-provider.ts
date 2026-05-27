import type { EnginePulseEvent } from "#/lib/symm/events";
import {
	isEnginePulseEvent,
	isPredictionFeedback,
	type PredictionFeedback,
} from "#/lib/symm/events";
import {
	isSignalSource,
	type SignalSource,
} from "#/lib/symm/signal-confidence";

export type PredictionSeriesKind = "predicted" | "actual" | "error";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	source: SignalSource;
	x: number;
	value: number;
};

type ReadingSink = (reading: PredictionReading) => void;

const MAX_BUFFER = 1200;

const returnToPercent = (value: number) => value * 100;

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

const forecastKey = (source: string, symbol: string, due: number) =>
	`${source}|${symbol}|${due}`;

class PredictionsDataProviderImpl {
	private sink: ReadingSink | null = null;
	private pulse: EnginePulseEvent | undefined;
	private openForecasts = new Set<string>();
	private buffer: PredictionReading[] = [];
	private listeners = new Set<() => void>();

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

	private emit(
		kind: PredictionSeriesKind,
		source: string,
		due: number,
		value: number,
	) {
		if (
			!isSignalSource(source) ||
			!Number.isFinite(value) ||
			!Number.isFinite(due)
		) {
			return;
		}

		this.push({
			kind,
			source,
			x: due,
			value: returnToPercent(value),
		});
	}

	ingestFeedback(raw: unknown) {
		if (!isPredictionFeedback(raw)) {
			return;
		}

		const predictedAt =
			timeSec(raw.DueAt) ?? timeSec(raw.PredictedAt) ?? timeSec(raw.SettledAt);
		const settledAt =
			timeSec(raw.SettledAt) ?? timeSec(raw.DueAt) ?? timeSec(raw.PredictedAt);

		if (predictedAt === undefined || settledAt === undefined) {
			return;
		}

		const key = forecastKey(raw.Source, raw.Symbol, predictedAt);
		const hadOpen = this.openForecasts.has(key);

		this.openForecasts.delete(key);

		if (!hadOpen) {
			this.emit("predicted", raw.Source, predictedAt, raw.PredictedReturn);
		}

		this.emit("actual", raw.Source, settledAt, raw.ActualReturn);
		this.emit("error", raw.Source, settledAt, raw.Error);
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

		const symbol = typeof row.symbol === "string" ? row.symbol : "";
		const due = timeSec(row.due_at) ?? timeSec(row.ts);

		if (due === undefined) {
			return;
		}

		const key = forecastKey(row.source, symbol, due);

		if (this.openForecasts.has(key)) {
			return;
		}

		this.openForecasts.add(key);
		this.emit("predicted", row.source, due, row.value);
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
};

export type { PredictionFeedback };
