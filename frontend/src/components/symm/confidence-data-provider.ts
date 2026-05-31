export type ConfidenceFactor = {
	name: string;
	value: number;
};

export type ConfidenceRow = {
	source: string;
	confidence: number;
	count?: number;
	factors?: ConfidenceFactor[];
};

type SourceSink = (row: ConfidenceRow) => void;

const sourceAliases: Record<string, string> = {
	basis: "liquidity",
	pump: "pumpdump",
	depth: "depthflow",
	sent: "sentiment",
};

const normalizeSource = (source: string): string =>
	sourceAliases[source] ?? source;

const normalizeFactors = (raw: unknown): ConfidenceFactor[] | undefined => {
	if (!Array.isArray(raw)) {
		return undefined;
	}

	const factors: ConfidenceFactor[] = [];

	for (const entry of raw) {
		if (typeof entry !== "object" || entry === null) {
			continue;
		}

		const factor = entry as Record<string, unknown>;

		if (typeof factor.name !== "string" || typeof factor.value !== "number") {
			continue;
		}

		factors.push({ name: factor.name, value: factor.value });
	}

	return factors.length > 0 ? factors : undefined;
};

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
	private latest = new Map<string, ConfidenceRow>();

	registerSource(source: string, sink: SourceSink) {
		const normalized = normalizeSource(source);
		const row = this.latest.get(normalized);

		if (row !== undefined) {
			sink(row);
		}

		this.sinks.set(normalized, sink);

		return () => {
			this.sinks.delete(normalized);
		};
	}

	snapshot(): ReadonlyMap<string, ConfidenceRow> {
		return this.latest;
	}

	ingest(raw: unknown) {
		if (!isConfidenceRow(raw)) {
			return;
		}

		const source = normalizeSource(raw.source);
		const row: ConfidenceRow = {
			source,
			confidence: raw.confidence,
			count: raw.count,
			factors:
				raw.factors ??
				normalizeFactors((raw as Record<string, unknown>).factors),
		};

		this.latest.set(source, row);
		this.sinks.get(source)?.(row);
	}

	ingestSnapshot(raw: unknown) {
		if (typeof raw !== "object" || raw === null) {
			return;
		}

		for (const [source, confidence] of Object.entries(raw)) {
			if (typeof confidence !== "number") {
				continue;
			}

			const normalized = normalizeSource(source);
			const previous = this.latest.get(normalized);

			this.ingest({
				source,
				confidence,
				factors: previous?.factors,
			});
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

export const formatConfidenceFactor = (factor: ConfidenceFactor): string =>
	`${factor.name}=${factor.value.toFixed(4)}`;
