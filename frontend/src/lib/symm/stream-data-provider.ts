export type StreamSink = (payload: unknown) => void;

/*
StreamDataProvider routes telemetry events to schema-driven renderers by stream id.
*/
class StreamDataProviderImpl {
	private latest = new Map<string, unknown>();
	private sinks = new Map<string, Set<StreamSink>>();

	subscribe(stream: string, sink: StreamSink) {
		const normalized = stream.trim();

		if (normalized.length === 0) {
			return () => undefined;
		}

		const latest = this.latest.get(normalized);

		if (latest !== undefined) {
			sink(latest);
		}

		let streamSinks = this.sinks.get(normalized);

		if (streamSinks === undefined) {
			streamSinks = new Set();
			this.sinks.set(normalized, streamSinks);
		}

		streamSinks.add(sink);

		return () => {
			streamSinks?.delete(sink);

			if (streamSinks?.size === 0) {
				this.sinks.delete(normalized);
			}
		};
	}

	snapshot(stream: string): unknown {
		return this.latest.get(stream.trim());
	}

	ingest(stream: string, payload: unknown) {
		const normalized = stream.trim();

		if (normalized.length === 0) {
			return;
		}

		this.latest.set(normalized, payload);
		const streamSinks = this.sinks.get(normalized);

		if (streamSinks === undefined) {
			return;
		}

		for (const sink of streamSinks) {
			sink(payload);
		}
	}

	reset() {
		this.latest.clear();
		this.sinks.clear();
	}
}

export const StreamDataProvider = new StreamDataProviderImpl();

export const createStreamDataProvider = () => new StreamDataProviderImpl();

export const readPayloadPath = (payload: unknown, path: string): unknown => {
	if (path.trim().length === 0) {
		return undefined;
	}

	const segments = path.split(".").filter((segment) => segment.length > 0);
	let current: unknown = payload;

	for (const segment of segments) {
		if (typeof current !== "object" || current === null) {
			return undefined;
		}

		current = (current as Record<string, unknown>)[segment];
	}

	return current;
};

export const readHeightMatrix = (
	payload: unknown,
	heightKey: string,
): number[][] | undefined => {
	const matrix = readPayloadPath(payload, heightKey);

	if (!Array.isArray(matrix)) {
		return undefined;
	}

	return matrix.map((row) => {
		if (!Array.isArray(row)) {
			return [];
		}

		return row.map((value) =>
			typeof value === "number" && Number.isFinite(value) ? value : 0,
		);
	});
};
