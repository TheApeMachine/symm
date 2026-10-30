import { decodeJsonBytes, tryDecodeJsonBytes } from "#/lib/json-bytes";

type ReadingsCache = {
	index: string[];
	layers: Uint8Array[];
	size: number;
	value: Array<{ suffix: string; payload: unknown }>;
};

const readingsCache = new WeakMap<Value, ReadingsCache>();

export class Value {
	id: string;
	epoch: bigint;
	tick: bigint;
	seqIdx: bigint;
	timestamp: bigint;
	origin: string;
	role: string;
	scope: string;
	prefix: string;
	layers: Uint8Array[];
	index: string[];

	/*
	Total wire size of the frame this value was decoded from, set by the
	decoder's batch reader to advance the cursor. Zero for constructed values.
	*/
	byteLength = 0;

	constructor(
		init: {
			id?: string;
			epoch?: bigint;
			tick?: bigint;
			seqIdx?: bigint;
			timestamp?: bigint;
			origin?: string;
			role?: string;
			scope?: string;
			prefix?: string;
			layers?: Uint8Array[];
			index?: string[];
		} = {},
	) {
		this.id = init.id ?? "";
		this.epoch = init.epoch ?? 0n;
		this.tick = init.tick ?? 0n;
		this.seqIdx = init.seqIdx ?? 0n;
		this.timestamp = init.timestamp ?? 0n;
		this.origin = init.origin ?? "";
		this.role = init.role ?? "";
		this.scope = init.scope ?? "";
		this.prefix = init.prefix ?? "";
		this.layers = init.layers ?? [];
		this.index = init.index ?? [];
	}

	lookup(keys: string[]): unknown[] {
		const layers: unknown[] = [];

		for (let i = 0; i < this.index.length; i++) {
			const index = this.index[i];
			if (!keys.some((key) => index.startsWith(`${this.prefix}/${key}`))) {
				continue;
			}

			layers.push(decodeJsonBytes(this.layers[i]));
		}

		return layers;
	}

	/*
	reading returns one layer decoded at exactly prefix/key.
	*/
	reading(key: string): unknown {
		const address = `${this.prefix}/${key}`;

		for (let i = 0; i < this.index.length; i++) {
			if (this.index[i] !== address) continue;
			return tryDecodeJsonBytes(this.layers[i]);
		}

		return undefined;
	}

	/*
	metric reads one scalar layer: the layer whose index is exactly
	prefix/key, decoded and returned with its first element. Signal and
	reading layers arrive as JSON arrays of floats (the Go Append call's
	[]float64), so a caller that wants one metric names one key.
	*/
	metric(key: string): unknown {
		const payload = this.reading(key);
		if (payload === undefined) return undefined;
		return Array.isArray(payload) ? payload[0] : payload;
	}

	/*
	readings returns every layer of the value as its decoded JSON payload
	paired with the index suffix after the prefix. Signal layers arrive as
	JSON arrays of floats (the Go Append call's []float64); object layers
	arrive decoded whole. Layers that do not decode JSON (gob payloads the
	frontend never reads) are skipped.
	*/
	readings(): Array<{ suffix: string; payload: unknown }> {
		const size = this.index.length + this.layers.length;
		const cached = readingsCache.get(this);
		if (
			cached &&
			cached.index === this.index &&
			cached.layers === this.layers &&
			cached.size === size
		) {
			return cached.value;
		}

		const found: Array<{ suffix: string; payload: unknown }> = [];
		const root = `${this.prefix}/`;

		for (let i = 0; i < this.index.length; i++) {
			const index = this.index[i];
			if (!index.startsWith(root)) continue;

			const payload = tryDecodeJsonBytes(this.layers[i]);
			if (payload === undefined) continue;

			found.push({ suffix: index.slice(root.length), payload });
		}

		readingsCache.set(this, {
			index: this.index,
			layers: this.layers,
			size,
			value: found,
		});

		return found;
	}

	/*
	Metrics collects the value's scalar readings keyed by the index suffix
	after the prefix — the kernel's metric name, or a training frame's
	metric name. Layers whose payload is not a single float are skipped.
	*/
	metrics(): Record<string, number> {
		const out: Record<string, number> = {};

		for (const { suffix, payload } of this.readings()) {
			const first = Array.isArray(payload) ? payload[0] : payload;
			if (typeof first !== "number" || !Number.isFinite(first)) continue;
			if (suffix === "") continue;
			out[suffix] = first;
		}

		return out;
	}

	/*
	Provenance collects the value's string readings keyed by index suffix —
	a training frame carries token paths, stage blockers, and event tags
	alongside its float metrics.
	*/
	provenance(): Record<string, string> {
		const out: Record<string, string> = {};

		for (const { suffix, payload } of this.readings()) {
			const first = Array.isArray(payload) ? payload[0] : payload;
			if (typeof first !== "string" || suffix === "") continue;
			out[suffix] = first;
		}

		return out;
	}

	/*
	layersUnder returns every layer whose index carries key, as the decoded
	JSON payload paired with the index suffix after the key. Signal layers
	arrive as JSON arrays of floats (the Go Append call's []float64); region
	layers arrive keyed by metric name in the suffix.
	*/
	layersUnder(key: string): Array<{ suffix: string; payload: unknown }> {
		const found: Array<{ suffix: string; payload: unknown }> = [];

		for (let i = 0; i < this.index.length; i++) {
			const index = this.index[i];
			const at = index.lastIndexOf(key);

			if (at < 0) {
				continue;
			}

			const payload = tryDecodeJsonBytes(this.layers[i]);
			if (payload === undefined) continue;

			found.push({
				suffix: index.slice(at + key.length).replace(/^\/+/, ""),
				payload,
			});
		}

		return found;
	}
}

export const layerRelativeToPrefix = (
	index: string,
	prefix: string,
): string | null => {
	const root = `${prefix}/`;
	if (!index.startsWith(root)) return null;
	return index.slice(root.length);
};

/*
mergeWorkspaceAggregate unions workspace aggregate layers by path below the
frame prefix. Incoming layers win on conflict; prior layers stay until the
same relative path appears on a later tick or sparse fan-out.
*/
export const mergeWorkspaceAggregate = (
	prior: Value | undefined,
	incoming: Value,
): Value => {
	if (!prior) return incoming;

	const byRelative = new Map<string, Uint8Array>();

	for (let i = 0; i < prior.index.length; i++) {
		const rel = layerRelativeToPrefix(prior.index[i], prior.prefix);
		if (rel === null) continue;
		byRelative.set(rel, prior.layers[i]);
	}

	for (let i = 0; i < incoming.index.length; i++) {
		const rel = layerRelativeToPrefix(incoming.index[i], incoming.prefix);
		if (rel === null) continue;
		byRelative.set(rel, incoming.layers[i]);
	}

	if (byRelative.size === 0) return incoming;

	const index: string[] = [];
	const layers: Uint8Array[] = [];
	for (const [rel, layer] of byRelative) {
		index.push(`${incoming.prefix}/${rel}`);
		layers.push(layer);
	}

	return new Value({
		id: incoming.id,
		epoch: incoming.epoch,
		tick: incoming.tick,
		seqIdx: incoming.seqIdx,
		timestamp: incoming.timestamp,
		origin: incoming.origin,
		role: incoming.role,
		scope: incoming.scope,
		prefix: incoming.prefix,
		index,
		layers,
	});
};

export const workspaceAggregateEqual = (
	left: Value | undefined,
	right: Value | undefined,
): boolean => {
	if (left === right) return true;
	if (!left || !right) return false;
	if (
		left.tick !== right.tick ||
		left.prefix !== right.prefix ||
		left.index.length !== right.index.length
	) {
		return false;
	}
	for (let i = 0; i < left.index.length; i++) {
		if (left.index[i] !== right.index[i]) return false;
		const a = left.layers[i];
		const b = right.layers[i];
		if (a === b) continue;
		if (!a || !b || a.byteLength !== b.byteLength) return false;
		for (let j = 0; j < a.byteLength; j++) {
			if (a[j] !== b[j]) return false;
		}
	}
	return true;
};
