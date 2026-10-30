/*
encodeValueWire serializes one Value the way the Go side does: fixed 48-byte
header, then origin, role, scope, layer count, the full prefix, and each
layer as an index string plus a byte payload.
*/
export function encodeValueWire({
	id = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]),
	tick = 100n,
	seqIdx = 1n,
	timestamp = 1718028000000000000n,
	epoch = 0n,
	origin = "training",
	role = "frame",
	scope = "BTC/USD",
	layers = [] as Array<{
		index: string;
		json?: unknown;
		raw?: Uint8Array;
	}>,
}) {
	const textEncoder = new TextEncoder();
	const originBytes = textEncoder.encode(origin);
	const roleBytes = textEncoder.encode(role);
	const scopeBytes = textEncoder.encode(scope);

	/*
	The Go prefix is origin/role/scope/epoch/tick: layers address themselves
	below it, exactly like data.NewValue + Append build it.
	*/
	const prefix = `${origin}/${role}/${scope}/${epoch.toString().padStart(19, "0")}/${tick.toString().padStart(19, "0")}`;
	const prefixBytes = textEncoder.encode(prefix);

	let size =
		48 +
		1 +
		originBytes.length +
		1 +
		roleBytes.length +
		1 +
		scopeBytes.length +
		2 +
		2 +
		prefixBytes.length;
	const encodedLayers: Array<{
		idxBytes: Uint8Array;
		payloadBytes: Uint8Array;
	}> = [];

	for (const l of layers) {
		const idxBytes = textEncoder.encode(l.index);
		const payloadBytes =
			l.json !== undefined
				? textEncoder.encode(JSON.stringify(l.json))
				: (l.raw ?? new Uint8Array(0));

		encodedLayers.push({ idxBytes, payloadBytes });
		size += 2 + idxBytes.length + 4 + payloadBytes.length;
	}

	const buf = new Uint8Array(size);
	const dv = new DataView(buf.buffer);

	buf.set(id, 0);
	dv.setBigUint64(16, tick, true);
	dv.setBigUint64(24, seqIdx, true);
	dv.setBigUint64(32, timestamp, true);
	dv.setBigUint64(40, epoch, true);

	let offset = 48;
	buf[offset++] = originBytes.length;
	buf.set(originBytes, offset);
	offset += originBytes.length;

	buf[offset++] = roleBytes.length;
	buf.set(roleBytes, offset);
	offset += roleBytes.length;

	buf[offset++] = scopeBytes.length;
	buf.set(scopeBytes, offset);
	offset += scopeBytes.length;

	dv.setUint16(offset, layers.length, true);
	offset += 2;

	dv.setUint16(offset, prefixBytes.length, true);
	offset += 2;
	buf.set(prefixBytes, offset);
	offset += prefixBytes.length;

	for (const { idxBytes, payloadBytes } of encodedLayers) {
		dv.setUint16(offset, idxBytes.length, true);
		offset += 2;
		buf.set(idxBytes, offset);
		offset += idxBytes.length;

		dv.setUint32(offset, payloadBytes.length, true);
		offset += 4;
		buf.set(payloadBytes, offset);
		offset += payloadBytes.length;
	}

	return buf;
}
