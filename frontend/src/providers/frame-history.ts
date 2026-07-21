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

/*
FrameHistory retains ordered observations for backend streams whose ticks are
deltas rather than complete UI history. Focused measurement entities retain the
visible temporal budget, cross-sectional entities retain their latest row, and
timestamp replay updates observations instead of manufacturing duplicate events.
*/
export class FrameHistory {
	private readonly streams = new Map<string, Map<string, TemporalRecord[]>>();

	/*
	The capacity and focus suppliers are evaluated on every update so retention
	follows the rendering budget and only the symbol temporal charts can inspect.
	*/
	constructor(
		private readonly capacity: () => number,
		private readonly focus: () => string = () => "",
	) {}

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
			let lower = 0;
			let upper = series.length;

			while (lower < upper) {
				const middle = Math.floor((lower + upper) / 2);

				if ((series[middle]?.at ?? "") < timestamp) {
					lower = middle + 1;
					continue;
				}

				upper = middle;
			}

			if (series[lower]?.at === timestamp) {
				series[lower] = row as TemporalRecord;
			} else {
				series.splice(lower, 0, row as TemporalRecord);
			}

			entities.set(identity, series);
			const limit =
				policy.retention === "focus" && row.symbol === focusSymbol
					? capacity
					: 1;
			series.splice(0, Math.max(0, series.length - limit));
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
	values returns a flat projection whose observations remain oldest-first within
	each entity, keeping history ownership out of individual UI components.
	*/
	values(stream: string): TemporalRecord[] {
		return [...(this.streams.get(stream)?.values() ?? [])].flat();
	}

	/*
	latest returns the newest observation for every entity in a stream so
	cross-sectional views do not repeatedly scan full temporal history.
	*/
	latest(stream: string): TemporalRecord[] {
		return [...(this.streams.get(stream)?.values() ?? [])].flatMap((series) => {
			const row = series.at(-1);

			return row === undefined ? [] : [row];
		});
	}
}
