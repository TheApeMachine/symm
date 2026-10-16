export interface Metric {
	name: string;
	raw: number;
	snm?: number;
}

export interface Tag {
	name: string;
	value: string;
}

export interface NamedNumber {
	name: string;
	value: number;
}

export class Value {
	id: string;
	tick: bigint;
	seqIdx: bigint;
	timestamp: bigint;
	origin: string;
	role: string;
	scope: string;
	layers: Uint8Array[];
	index: string[];

	metrics: Metric[];
	metadata: NamedNumber[];
	provenance: Tag[];
	snr: number;
	snrDefined: boolean;

	constructor(init: {
		id?: string;
		tick?: bigint;
		seqIdx?: bigint;
		timestamp?: bigint;
		origin?: string;
		role?: string;
		scope?: string;
		layers?: Uint8Array[];
		index?: string[];
		metrics?: Metric[];
		metadata?: NamedNumber[];
		provenance?: Tag[];
		snr?: number;
		snrDefined?: boolean;
	} = {}) {
		this.id = init.id ?? "";
		this.tick = init.tick ?? 0n;
		this.seqIdx = init.seqIdx ?? 0n;
		this.timestamp = init.timestamp ?? 0n;
		this.origin = init.origin ?? "";
		this.role = init.role ?? "";
		this.scope = init.scope ?? "";
		this.layers = init.layers ?? [];
		this.index = init.index ?? [];
		this.metrics = init.metrics ?? [];
		this.metadata = init.metadata ?? [];
		this.provenance = init.provenance ?? [];
		this.snr = init.snr ?? 0.0;
		this.snrDefined = init.snrDefined ?? false;
	}

	metric(name: string): number | undefined {
		for (const m of this.metrics) {
			if (m.name === name) return m.raw;
		}
		return undefined;
	}
}
