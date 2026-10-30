import { Value } from "#/types/value";

const textDecoder = new TextDecoder();

/*
WireValue is the structured-cloneable hub frame. The websocket worker parses
binary batches into WireValue[]; the main thread turns them into Value instances
for ingest.
*/
export type WireValue = {
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
	byteLength: number;
};

export const valueFromWire = (wire: WireValue): Value => {
	const value = new Value({
		id: wire.id,
		epoch: wire.epoch,
		tick: wire.tick,
		seqIdx: wire.seqIdx,
		timestamp: wire.timestamp,
		origin: wire.origin,
		role: wire.role,
		scope: wire.scope,
		prefix: wire.prefix,
		layers: wire.layers,
		index: wire.index,
	});
	value.byteLength = wire.byteLength;
	return value;
};

const parseOneWireValue = (
	bytes: Uint8Array,
	byteOffset = 0,
): WireValue | null => {
	const remaining = bytes.byteLength - byteOffset;
	if (remaining < 53) return null;

	const view = new DataView(
		bytes.buffer,
		bytes.byteOffset + byteOffset,
		remaining,
	);

	const idBytes = bytes.subarray(byteOffset, byteOffset + 16);
	const id = Array.from(idBytes)
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");

	const tick = view.getBigUint64(16, true);
	const seqIdx = view.getBigUint64(24, true);
	const timestamp = view.getBigUint64(32, true);
	const epoch = view.getBigUint64(40, true);

	let cursor = byteOffset + 48;

	if (cursor >= bytes.byteLength) return null;
	const originLen = bytes[cursor++];

	if (cursor + originLen > bytes.byteLength) return null;
	const origin = textDecoder.decode(bytes.subarray(cursor, cursor + originLen));
	cursor += originLen;

	if (cursor >= bytes.byteLength) return null;
	const roleLen = bytes[cursor++];

	if (cursor + roleLen > bytes.byteLength) return null;
	const role = textDecoder.decode(bytes.subarray(cursor, cursor + roleLen));
	cursor += roleLen;

	if (cursor >= bytes.byteLength) return null;
	const scopeLen = bytes[cursor++];

	if (cursor + scopeLen > bytes.byteLength) return null;
	const scope = textDecoder.decode(bytes.subarray(cursor, cursor + scopeLen));
	cursor += scopeLen;

	if (cursor + 2 > bytes.byteLength) return null;
	const numLayers = view.getUint16(cursor - byteOffset, true);
	cursor += 2;

	if (cursor + 2 > bytes.byteLength) return null;
	const prefixLen = view.getUint16(cursor - byteOffset, true);
	cursor += 2;

	if (cursor + prefixLen > bytes.byteLength) return null;
	const prefix = textDecoder.decode(bytes.subarray(cursor, cursor + prefixLen));
	cursor += prefixLen;

	const index: string[] = [];
	const layers: Uint8Array[] = [];

	for (let i = 0; i < numLayers; i++) {
		if (cursor + 2 > bytes.byteLength) return null;
		const idxLen = view.getUint16(cursor - byteOffset, true);
		cursor += 2;
		if (cursor + idxLen > bytes.byteLength) return null;
		const layerIndex =
			idxLen > 0
				? textDecoder.decode(bytes.subarray(cursor, cursor + idxLen))
				: "";
		cursor += idxLen;

		if (cursor + 4 > bytes.byteLength) return null;
		const layerLen = view.getUint32(cursor - byteOffset, true);
		cursor += 4;
		if (cursor + layerLen > bytes.byteLength) return null;
		const layerBytes = bytes.subarray(cursor, cursor + layerLen);
		cursor += layerLen;

		index.push(layerIndex);
		layers.push(new Uint8Array(layerBytes));
	}

	const totalSize = cursor - byteOffset;

	return {
		id,
		tick,
		seqIdx,
		timestamp,
		epoch,
		origin,
		role,
		scope,
		prefix,
		layers,
		index,
		byteLength: totalSize,
	};
};

/*
parseOneValue decodes one wire frame into the one Value the whole frontend
reads. byteLength records the consumed wire size so parseValues can advance
through a batch.
*/
export const parseOneValue = (
	bytes: Uint8Array,
	byteOffset = 0,
): Value | null => {
	const wire = parseOneWireValue(bytes, byteOffset);
	if (!wire) return null;
	return valueFromWire(wire);
};

/*
parseWireValues parses one websocket message into clone-safe wire records.
The worker uses this so binary decoding stays off the UI thread.
*/
export const parseWireValues = (bytes: Uint8Array): WireValue[] | null => {
	if (bytes.byteLength < 53) return null;

	const list: WireValue[] = [];
	let offset = 0;

	while (offset < bytes.byteLength) {
		const wire = parseOneWireValue(bytes, offset);
		if (!wire || wire.byteLength === 0) {
			break;
		}
		list.push(wire);
		offset += wire.byteLength;
	}

	return list.length > 0 ? list : null;
};

/*
parseValues parses one websocket message: raw, back-to-back Value frames
exactly as the hub enqueues them (io.Copy(hub, value) hands over one full
serialization per Write — no batch header). A frame that does not decode
ends the message; values parsed before it are kept.
*/
export const parseValues = (bytes: Uint8Array): Value[] | null => {
	const wire = parseWireValues(bytes);
	if (!wire) return null;
	return wire.map(valueFromWire);
};
