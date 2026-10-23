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

		for (const index of this.index) {
			if (keys.some((key) => index.startsWith(`${this.prefix}/${key}`))) {
				layers.push(
					JSON.parse(
						new TextDecoder().decode(this.layers[this.index.indexOf(index)]),
					),
				);
			}
		}

		return layers;
	}

	/*
	Metric reads one scalar layer: the layer whose index is exactly
	prefix/key, decoded and returned with its first element. Signal and
	reading layers arrive as JSON arrays of floats (the Go Append call's
	[]float64), so a caller that wants one metric names one key.
	*/
	metric(key: string): unknown {
		for (let i = 0; i < this.index.length; i++) {
			if (this.index[i] !== `${this.prefix}/${key}`) continue;

			let payload: unknown;
			try {
				payload = JSON.parse(new TextDecoder().decode(this.layers[i]));
			} catch {
				return undefined;
			}

			return Array.isArray(payload) ? payload[0] : payload;
		}

		return undefined;
	}

	/*
	readings returns every layer of the value as its decoded JSON payload
	paired with the index suffix after the prefix. Signal layers arrive as
	JSON arrays of floats (the Go Append call's []float64); object layers
	arrive decoded whole. Layers that do not decode JSON (gob payloads the
	frontend never reads) are skipped.
	*/
	readings(): Array<{ suffix: string; payload: unknown }> {
		const found: Array<{ suffix: string; payload: unknown }> = [];
		const root = `${this.prefix}/`;

		for (let i = 0; i < this.index.length; i++) {
			const index = this.index[i];
			if (!index.startsWith(root)) continue;

			let payload: unknown;
			try {
				payload = JSON.parse(new TextDecoder().decode(this.layers[i]));
			} catch {
				continue;
			}

			found.push({ suffix: index.slice(root.length), payload });
		}

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

			let payload: unknown;
			try {
				payload = JSON.parse(new TextDecoder().decode(this.layers[i]));
			} catch {
				continue;
			}

			found.push({
				suffix: index.slice(at + key.length).replace(/^\/+/, ""),
				payload,
			});
		}

		return found;
	}
}
