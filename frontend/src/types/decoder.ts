import { Value } from "#/types/value";

const textDecoder = new TextDecoder();

/*
parseOneValue decodes one wire frame into the one Value the whole frontend
reads. byteLength records the consumed wire size so parseValues can advance
through a batch.
*/
export const parseOneValue = (
	bytes: Uint8Array,
	byteOffset = 0,
): Value | null => {
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

	const layers: Array<{ index: string; bytes: Uint8Array }> = [];
	for (let i = 0; i < numLayers; i++) {
		if (cursor + 2 > bytes.byteLength) return null;
		const idxLen = view.getUint16(cursor - byteOffset, true);
		cursor += 2;
		if (cursor + idxLen > bytes.byteLength) return null;
		const index =
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

		layers.push({ index, bytes: layerBytes });
	}

	const totalSize = cursor - byteOffset;
	const value = new Value({
		id,
		tick,
		seqIdx,
		timestamp,
		epoch,
		origin,
		role,
		scope,
		prefix,
		layers: layers.map((layer) => layer.bytes),
		index: layers.map((layer) => layer.index),
	});

	value.byteLength = totalSize;
	return value;
};

/*
parseValues parses one websocket message: raw, back-to-back Value frames
exactly as the hub enqueues them (io.Copy(hub, value) hands over one full
serialization per Write — no batch header). A frame that does not decode
ends the message; values parsed before it are kept.
*/
export const parseValues = (bytes: Uint8Array): Value[] | null => {
	if (bytes.byteLength < 53) return null;

	const list: Value[] = [];
	let offset = 0;

	while (offset < bytes.byteLength) {
		const val = parseOneValue(bytes, offset);
		if (!val || val.byteLength === 0) {
			break;
		}
		list.push(val);
		offset += val.byteLength;
	}

	return list.length > 0 ? list : null;
};
