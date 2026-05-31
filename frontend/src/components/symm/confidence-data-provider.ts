export type ConfidenceRow = {
	source: string;
	confidence: number;
	count?: number;
};

type SourceSink = (confidence: number) => void;

const sourceAliases: Record<string, string> = {
	basis: "liquidity",
	pump: "pumpdump",
	depth: "depthflow",
	sent: "sentiment",
};

const normalizeSource = (source: string): string =>
	sourceAliases[source] ?? source;

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
		const normalized = normalizeSource(source);
		const confidence = this.latest.get(normalized);

		if (confidence !== undefined) {
			sink(confidence);
		}

		this.sinks.set(normalized, sink);

		return () => {
			this.sinks.delete(normalized);
		};
	}

	snapshot(): ReadonlyMap<string, number> {
		return this.latest;
	}

	ingest(raw: unknown) {
		if (!isConfidenceRow(raw)) {
			return;
		}

		const source = normalizeSource(raw.source);

		this.latest.set(source, raw.confidence);
		this.sinks.get(source)?.(raw.confidence);
	}

	ingestSnapshot(raw: unknown) {
		if (typeof raw !== "object" || raw === null) {
			return;
		}

		for (const [source, confidence] of Object.entries(raw)) {
			this.ingest({ source, confidence });
		}
	}

	reset() {
		this.sinks.clear();
		this.latest.clear();
	}
}

const shared = createConfidenceDataProviderImpl();

export const createConfidenceDataProvider = () =>
	createConfidenceDataProviderImpl();

function createConfidenceDataProviderImpl() {
	const impl = new ConfidenceDataProviderImpl();

	return {
		registerSource: (source: string, sink: SourceSink) =>
			impl.registerSource(source, sink),
		snapshot: () => impl.snapshot(),
		ingest: (raw: unknown) => impl.ingest(raw),
		ingestSnapshot: (raw: unknown) => impl.ingestSnapshot(raw),
		reset: () => impl.reset(),
	};
}

export type ConfidenceStore = ReturnType<typeof createConfidenceDataProvider>;

export const ConfidenceDataProvider = shared;
