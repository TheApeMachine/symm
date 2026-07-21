/*
TemporalRecord is the minimum backend observation retained by FrameHistory.
*/
type TemporalRecord = Record<string, unknown> & { at: string };

/*
TemporalPolicy defines stable entity identity and required wire fields.
*/
type TemporalPolicy = {
	identity: readonly string[];
	required: readonly string[];
};

const TEMPORAL_POLICIES: Record<string, TemporalPolicy> = {
	measurements: {
		identity: ["symbol", "source", "metric", "side", "subject", "stream"],
		required: ["symbol", "source"],
	},
	causal: { identity: ["symbol"], required: ["symbol"] },
	manifold: { identity: ["symbol"], required: ["symbol"] },
	resonance: { identity: ["symbol"], required: ["symbol"] },
};

/*
FrameHistory retains ordered observations for backend streams whose ticks are
deltas rather than complete UI history. Each entity owns its own capacity so a
busy symbol cannot evict quieter symbols, and timestamp replay updates the
existing observation instead of manufacturing duplicate events.
*/
export class FrameHistory {
	private readonly streams = new Map<string, Map<string, TemporalRecord[]>>();

	/*
	The capacity supplier is evaluated on every update so retention follows the
	current rendering budget when the viewport changes.
	*/
	constructor(private readonly capacity: () => number) {}

	/*
	retain merges one wire value into its configured temporal stream and returns
	the entire ordered history. Snapshot streams pass through untouched.
	*/
	retain(stream: string, value: unknown): unknown {
		const policy = TEMPORAL_POLICIES[stream];

		if (policy === undefined) {
			return value;
		}

		const rows = (Array.isArray(value) ? value : [value]) as unknown[];
		const capacity = this.capacity();

		if (!Number.isInteger(capacity) || capacity < 1) {
			throw new Error(`invalid ${stream} history capacity: ${capacity}`);
		}

		const entities =
			this.streams.get(stream) ?? new Map<string, TemporalRecord[]>();
		this.streams.set(stream, entities);

		for (const candidate of rows) {
			if (candidate === null || typeof candidate !== "object") {
				throw new Error(`${stream} history requires object rows`);
			}

			const row = candidate as Record<string, unknown>;

			for (const field of policy.required) {
				if (typeof row[field] !== "string" || row[field] === "") {
					throw new Error(`${stream} history row requires ${field}`);
				}
			}

			const timestamp = row.at;

			if (typeof timestamp !== "string" || timestamp === "") {
				throw new Error(`${stream} history row requires at`);
			}

			const identity = JSON.stringify(
				policy.identity.map((field) => row[field] ?? null),
			);
			const series = entities.get(identity) ?? [];
			const existing = series.findIndex((entry) => entry.at === timestamp);

			if (existing >= 0) {
				series[existing] = row as TemporalRecord;
			} else {
				const insertion = series.findIndex((entry) => entry.at > timestamp);
				series.splice(
					insertion < 0 ? series.length : insertion,
					0,
					row as TemporalRecord,
				);
			}

			entities.set(identity, series);
		}

		for (const series of entities.values()) {
			series.splice(0, Math.max(0, series.length - capacity));
		}

		return this.values(stream);
	}

	/*
	values returns one flat oldest-first projection for existing painters, which
	keeps history ownership out of individual React and canvas components.
	*/
	values(stream: string): TemporalRecord[] {
		return [...(this.streams.get(stream)?.values() ?? [])]
			.flat()
			.sort((left, right) => left.at.localeCompare(right.at));
	}
}
