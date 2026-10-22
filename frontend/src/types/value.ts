export class Value {
	id: string;
	epoch: bigint;
	tick: bigint;
	seqIdx: bigint;
	timestamp: bigint;
	origin: string;
	role: string;
	scope: string;
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
		this.layers = init.layers ?? [];
		this.index = init.index ?? [];
	}

	lookup(key: string): Record<string, unknown> | undefined {
		for (const index of this.index) {
			if (index.startsWith(key)) {
				const layer = this.layers[this.index.indexOf(index)];
				const parsed = JSON.parse(new TextDecoder().decode(layer));

				if (parsed) {
					return parsed;
				}
			}
		}
	}

	/*
	layersUnder returns every layer whose index contains key, as the decoded
	JSON payload paired with the index suffix after the key. Signal layers
	arrive as JSON arrays of floats (the Go Append call's []float64); training
	regions arrive keyed by metric name in the suffix.
	*/
	layersUnder(
		key: string,
	): Array<{ suffix: string; payload: unknown }> {
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
