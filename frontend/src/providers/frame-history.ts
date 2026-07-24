/*
TemporalRecord is the minimum backend observation retained by FrameHistory.
Epoch milliseconds drive ordering so noncanonical string timestamps cannot
regress chronology.
*/
type TemporalRecord = Record<string, unknown> & {
	at: string;
	atMs: number;
};

/*
TemporalPolicy defines stable entity identity and required wire fields.
*/
type TemporalPolicy = {
	identity: readonly string[];
	required: readonly string[];
	retention: "focus" | "latest";
};

/*
frameRows normalizes a retained projection or ordinary wire value into the
array shape consumed by painters.
*/
export const frameRows = <T>(value: unknown): T[] => {
	return (Array.isArray(value) ? value : value != null ? [value] : []) as T[];
};

const TEMPORAL_POLICIES: Record<string, TemporalPolicy> = {
	measurements: {
		identity: ["symbol", "source", "metric", "side", "subject", "stream"],
		required: ["symbol", "source"],
		retention: "focus",
	},
	causal: {
		identity: ["symbol"],
		required: ["symbol"],
		retention: "latest",
	},
	manifold: {
		identity: ["symbol"],
		required: ["symbol"],
		retention: "latest",
	},
	resonance: {
		identity: ["symbol"],
		required: ["symbol"],
		retention: "focus",
	},
	cognition: {
		identity: ["symbol"],
		required: ["symbol"],
		retention: "latest",
	},
};

const DEFAULT_ENTITY_LIMIT = 256;

/*
parseEpoch converts a wire timestamp into milliseconds since Unix epoch.
*/
const parseEpoch = (timestamp: string): number => {
	const ms = Date.parse(timestamp);

	if (!Number.isFinite(ms)) {
		throw new Error(`invalid temporal timestamp: ${timestamp}`);
	}

	return ms;
};

/*
FrameHistory retains ordered observations for backend streams whose ticks are
deltas rather than complete UI history. Entity cardinality is LRU-bounded per
stream, focused series retain the temporal budget, and projections are globally
time-ordered by epoch milliseconds.
*/
export class FrameHistory {
	private readonly streams = new Map<string, Map<string, TemporalRecord[]>>();
	private readonly touch = new Map<string, Map<string, number>>();
	private readonly entityLimit: number;
	private clock = 0;

	/*
	The capacity and focus suppliers are evaluated on every update so retention
	follows the rendering budget and only the symbol temporal charts can inspect.
	*/
	constructor(
		private readonly capacity: () => number,
		private readonly focus: () => string = () => "",
		entityLimit = DEFAULT_ENTITY_LIMIT,
	) {
		this.entityLimit = entityLimit;
	}

	/*
	retain merges one wire value into its configured temporal stream. Snapshot
	streams are intentionally ignored because their painters consume raw frames.
	*/
	retain(stream: string, value: unknown): void {
		const policy = TEMPORAL_POLICIES[stream];

		if (policy === undefined) {
			return;
		}

		const rows = (Array.isArray(value) ? value : [value]) as unknown[];
		const capacity = this.capacity();
		const focusSymbol = this.focus();

		if (!Number.isInteger(capacity) || capacity < 1) {
			throw new Error(`invalid ${stream} history capacity: ${capacity}`);
		}

		const entities =
			this.streams.get(stream) ?? new Map<string, TemporalRecord[]>();
		this.streams.set(stream, entities);
		const touches = this.touch.get(stream) ?? new Map<string, number>();
		this.touch.set(stream, touches);

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

			const atMs = parseEpoch(timestamp);
			const identity = JSON.stringify(
				policy.identity.map((field) => row[field] ?? null),
			);
			const series = entities.get(identity) ?? [];
			let lower = 0;
			let upper = series.length;

			while (lower < upper) {
				const middle = Math.floor((lower + upper) / 2);

				if ((series[middle]?.atMs ?? 0) < atMs) {
					lower = middle + 1;
					continue;
				}

				upper = middle;
			}

			const record = { ...row, at: timestamp, atMs } as TemporalRecord;

			if (series[lower]?.atMs === atMs) {
				series[lower] = record;
			} else {
				series.splice(lower, 0, record);
			}

			entities.set(identity, series);
			this.clock += 1;
			touches.set(identity, this.clock);

			const limit =
				policy.retention === "focus" && row.symbol === focusSymbol
					? capacity
					: 1;
			series.splice(0, Math.max(0, series.length - limit));
		}

		this.pruneFocus(stream, policy, focusSymbol, capacity);
		this.evict(entities, touches);
	}

	/*
	pruneFocus immediately shrinks non-focused series when focus changes.
	*/
	private pruneFocus(
		stream: string,
		policy: TemporalPolicy,
		focusSymbol: string,
		capacity: number,
	): void {
		const entities = this.streams.get(stream);

		if (entities === undefined) {
			return;
		}

		for (const [identity, series] of entities) {
			const latest = series.at(-1);
			const focused =
				policy.retention === "focus" && latest?.symbol === focusSymbol;
			const limit = focused ? capacity : 1;
			series.splice(0, Math.max(0, series.length - limit));

			if (series.length === 0) {
				entities.delete(identity);
				this.touch.get(stream)?.delete(identity);
			}
		}
	}

	/*
	evict drops the least-recently touched entities once cardinality exceeds
	the stream LRU bound.
	*/
	private evict(
		entities: Map<string, TemporalRecord[]>,
		touches: Map<string, number>,
	): void {
		while (entities.size > this.entityLimit) {
			let oldestKey = "";
			let oldestTouch = Number.POSITIVE_INFINITY;

			for (const [identity, stamp] of touches) {
				if (stamp < oldestTouch) {
					oldestTouch = stamp;
					oldestKey = identity;
				}
			}

			if (oldestKey === "") {
				return;
			}

			entities.delete(oldestKey);
			touches.delete(oldestKey);
		}
	}

	/*
	project returns the requested retained view for every target on one dispatch.
	The dispatcher memoizes this array so sibling painters share the projection.
	*/
	project(stream: string, input: "history" | "latest"): TemporalRecord[] {
		return input === "history" ? this.values(stream) : this.latest(stream);
	}

	/*
	values returns a globally time-ordered projection across every entity.
	*/
	values(stream: string): TemporalRecord[] {
		const rows = [...(this.streams.get(stream)?.values() ?? [])].flat();
		rows.sort((left, right) => left.atMs - right.atMs);
		return rows;
	}

	/*
	latest returns the newest observation for every entity in a stream so
	cross-sectional views do not repeatedly scan full temporal history.
	*/
	latest(stream: string): TemporalRecord[] {
		const rows = [...(this.streams.get(stream)?.values() ?? [])].flatMap(
			(series) => {
				const row = series.at(-1);

				return row === undefined ? [] : [row];
			},
		);
		rows.sort((left, right) => left.atMs - right.atMs);
		return rows;
	}
}
