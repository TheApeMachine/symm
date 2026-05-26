export type ConfidenceRow = {
	source: string;
	confidence: number;
	count?: number;
};

type SourceSink = (confidence: number) => void;

export const isConfidenceRow = (raw: unknown): raw is ConfidenceRow => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return typeof row.source === "string" && typeof row.confidence === "number";
};

/*
ConfidenceDataProvider routes hub mean-confidence readings to registered gauge sinks.
*/
class ConfidenceDataProviderImpl {
	private sinks = new Map<string, SourceSink>();
	private latest = new Map<string, number>();

	registerSource(source: string, sink: SourceSink) {
		this.sinks.set(source, sink);

		const confidence = this.latest.get(source);

		if (confidence !== undefined) {
			sink(confidence);
		}

		return () => {
			this.sinks.delete(source);
		};
	}

	snapshot(): ReadonlyMap<string, number> {
		return this.latest;
	}

	ingest(raw: unknown) {
		if (!isConfidenceRow(raw)) {
			return;
		}

		this.latest.set(raw.source, raw.confidence);
		this.sinks.get(raw.source)?.(raw.confidence);
	}
}

const shared = new ConfidenceDataProviderImpl();

export const ConfidenceDataProvider = {
	registerSource: (source: string, sink: SourceSink) =>
		shared.registerSource(source, sink),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
};
