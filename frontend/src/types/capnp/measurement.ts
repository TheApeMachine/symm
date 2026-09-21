import {
	ElementSize,
	ListReader,
	MessageReader,
	PointerTag,
	type StructSize,
	StructReader,
} from "@naeemo/capnp";

// Fix composite list element count decoding to conform to Cap'n Proto specification:
// A composite list tag word has the layout of a struct pointer, where the offset field (bits 2..31)
// stores the list's element count (requiring a right-shift of 2).
StructReader.prototype.getList = function (
	pointerIndex: number,
	_elementSize?: ElementSize,
	structSize?: StructSize,
): ListReader | undefined {
	const ptrOffset = this.wordOffset + this.dataWords + pointerIndex;
	const resolved = this.message.resolvePointer(this.segmentIndex, ptrOffset);
	if (!resolved) return undefined;
	const { segmentIndex, wordOffset, pointer } = resolved;
	if (pointer.tag !== PointerTag.LIST) return undefined;
	const listPtr = pointer;
	let targetOffset = wordOffset;
	let elementCount = listPtr.elementCount;
	let actualStructSize = structSize;
	const segment = this.message.getSegment(segmentIndex);
	if (listPtr.elementSize === ElementSize.COMPOSITE) {
		if (targetOffset < 0 || !segment || targetOffset >= segment.wordCount) return undefined;
		try {
			const tagWord = segment.getWord(targetOffset);
			elementCount = Number((tagWord >> 2n) & 0x3fffffffn);
			actualStructSize = {
				dataWords: Number((tagWord >> 32n) & 65535n),
				pointerCount: Number((tagWord >> 48n) & 65535n),
			};
			targetOffset += 1;
		} catch {
			return undefined;
		}
	}
	return new ListReader(
		this.message,
		segmentIndex,
		listPtr.elementSize,
		elementCount,
		actualStructSize,
		targetOffset,
	);
};

export enum EntityType {
	TICKER = 0,
	TRADE = 1,
	LEVEL3 = 2,
	INSTRUMENT = 3,
	ORDER = 4,
	BALANCE = 5,
	EXECUTION = 6,
	TRADE_HISTORY = 7,
	OPEN_ORDERS = 8,
	TRADE_BALANCE = 9,
	TRADE_VOLUME = 10,
}

export const ENTITY_TYPE_NAMES = [
	"ticker",
	"trade",
	"level3",
	"instrument",
	"order",
	"balance",
	"execution",
	"tradeHistory",
	"openOrders",
	"tradeBalance",
	"tradeVolume",
] as const;

export enum SourceType {
	PUBLIC = 0,
	PRIVATE = 1,
	LEVEL3 = 2,
	CORRELATION = 3,
	CSV = 4,
	DEPTHFLOW = 5,
	DERIVATIVES = 6,
	HAWKES = 7,
	LEADLAG = 8,
	LIQUIDITY = 9,
	MORPHOLOGY = 10,
	PUMPDUMP = 11,
	SENTIMENT = 12,
	TOXICITY = 13,
	CATEGORY = 14,
	COGNITION = 15,
	RESONANCE = 16,
	MANIFOLD = 17,
	TRAINING = 18,
}

export const SOURCE_TYPE_NAMES = [
	"public",
	"private",
	"level3",
	"correlation",
	"csv",
	"depthflow",
	"derivatives",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
	"pumpdump",
	"sentiment",
	"toxicity",
	"category",
	"cognition",
	"resonance",
	"manifold",
	"training",
] as const;

export enum UnitType {
	DIMENSIONLESS = 0,
	COUNT = 1,
	RATE = 2,
	DURATION = 3,
	PERCENT = 4,
	SECOND = 5,
	PER_SECOND = 6,
	NAT = 7,
}

export enum Timescale {
	INSTANTANEOUS = 0,
	PER_SECOND = 1,
	PER_MINUTE = 2,
	PER_HOUR = 3,
	PER_DAY = 4,
}

export interface WireMetric {
	name?: string;
	raw: number;
	normalized: number;
	standardized?: number;
	center?: number;
	scale?: number;
	unit?: UnitType;
	timescale?: Timescale;
}

export interface WireProvenance {
	name: string;
	value: string;
}

export interface WireMeasurement {
	id: string;
	source: string;
	symbol: string;
	tick: bigint;
	at: bigint;
	timestamp: bigint;
	entity: EntityType;
	maturity: number;
	snr: number;
	separation: number;
	metrics: WireMetric[];
	metadata: Record<string, string | number | boolean>;
	provenance: WireProvenance[];
}

const textDecoder = new TextDecoder();

export class WireMetricReader {
	constructor(private readonly reader: StructReader) {}

	get raw(): number {
		return this.reader.getFloat64(0);
	}

	get normalized(): number {
		return this.reader.getFloat64(8);
	}

	get standardized(): number {
		return this.reader.getFloat64(16);
	}

	get center(): number {
		return this.reader.getFloat64(24);
	}

	get scale(): number {
		return this.reader.getFloat64(32);
	}

	get unit(): UnitType {
		return this.reader.getUint16(40) as UnitType;
	}

	get timescale(): Timescale {
		return this.reader.getUint16(42) as Timescale;
	}

	toMetric(name?: string): WireMetric {
		return {
			name,
			raw: this.raw,
			normalized: this.normalized,
			standardized: this.standardized,
			center: this.center,
			scale: this.scale,
			unit: this.unit,
			timescale: this.timescale,
		};
	}
}

export class WireMeasurementReader {
	constructor(private readonly reader: StructReader) {}

	get epoch(): bigint {
		return this.reader.getInt64(0);
	}

	get tick(): bigint {
		return this.reader.getInt64(8);
	}

	get timestamp(): bigint {
		return this.reader.getInt64(16);
	}

	get entity(): EntityType {
		return this.reader.getUint16(24) as EntityType;
	}

	get source(): SourceType {
		return this.reader.getUint16(26) as SourceType;
	}

	get sourceName(): string {
		const idx = this.source;
		if (idx >= 0 && idx < SOURCE_TYPE_NAMES.length) {
			return SOURCE_TYPE_NAMES[idx];
		}
		return "unknown";
	}

	get snr(): number {
		return this.reader.getFloat64(32);
	}

	get maturity(): number {
		return this.reader.getFloat64(40);
	}

	get separation(): number {
		return this.reader.getFloat64(48);
	}

	get id(): string {
		const bytes = this.reader.getData(0);
		if (!bytes || bytes.length === 0) return "";
		return textDecoder.decode(bytes);
	}

	get label(): string {
		const bytes = this.reader.getData(1);
		if (!bytes || bytes.length === 0) return "";
		return textDecoder.decode(bytes);
	}

	get metrics(): WireMetric[] {
		const list = this.reader.getList(2);
		if (!list) return [];
		const count = list.length;
		const out: WireMetric[] = new Array(count);
		for (let i = 0; i < count; i++) {
			const struct = list.getStruct(i);
			if (!struct) continue;
			const metricReader = new WireMetricReader(struct);
			out[i] = metricReader.toMetric(`metric_${i}`);
		}
		return out;
	}

	get metadata(): Record<string, string | number | boolean> {
		const out: Record<string, string | number | boolean> = {};
		const mapStruct = this.reader.getStruct(3);
		if (!mapStruct) return out;

		const entriesList = mapStruct.getList(0);
		if (!entriesList) return out;

		const count = entriesList.length;
		for (let i = 0; i < count; i++) {
			const entryStruct = entriesList.getStruct(i);
			if (!entryStruct) continue;

			const key = entryStruct.getText(0) || "";
			const valStruct = entryStruct.getStruct(1);
			if (!valStruct) continue;

			const which = valStruct.getUint16(0);
			switch (which) {
				case 0: { // id (Data)
					const data = valStruct.getData(0);
					out[key] = data ? textDecoder.decode(data) : "";
					break;
				}
				case 1: { // text (Text)
					out[key] = valStruct.getText(0) || "";
					break;
				}
				case 2: { // int (Int64)
					out[key] = Number(valStruct.getInt64(8));
					break;
				}
				case 3: { // float (Float64)
					out[key] = valStruct.getFloat64(8);
					break;
				}
				case 4: { // bool (Bool)
					out[key] = (valStruct.getInt64(8) & 1n) !== 0n;
					break;
				}
			}
		}
		return out;
	}

	toMeasurement(): WireMeasurement {
		const meta = this.metadata;
		const provenance: WireProvenance[] = Object.entries(meta).map(
			([name, value]) => ({ name, value: String(value) }),
		);

		return {
			id: this.id,
			source: this.sourceName,
			symbol: this.label,
			tick: this.tick,
			at: this.epoch || this.timestamp,
			timestamp: this.timestamp,
			entity: this.entity,
			maturity: this.maturity,
			snr: this.snr,
			separation: this.separation,
			metrics: this.metrics,
			metadata: meta,
			provenance,
		};
	}
}

export function readWireMeasurement(buffer: ArrayBuffer | Uint8Array): WireMeasurement {
	const message = new MessageReader(buffer);
	const root = message.getRoot(7, 4);
	const reader = new WireMeasurementReader(root);
	return reader.toMeasurement();
}
