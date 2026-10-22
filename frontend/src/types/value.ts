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
}
