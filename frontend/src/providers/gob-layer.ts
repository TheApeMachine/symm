/*
gobLayer reads one encoding/gob value written by data.Value.Append.
A fresh encoder sends []byte as builtin id 5 and []float64 as the first
user type. Anything else is left for the raw readers.
*/

export type GobLayer =
	| { kind: "bytes"; bytes: Uint8Array }
	| { kind: "floats"; floats: number[] };

const readUint = (
	bytes: Uint8Array,
	at: number,
): { value: bigint; at: number } | null => {
	if (at >= bytes.length) return null;

	const first = bytes[at];

	if (first <= 0x7f) {
		return { value: BigInt(first), at: at + 1 };
	}

	const width = 256 - first;

	if (width < 1 || width > 8 || at + 1 + width > bytes.length) return null;

	let value = 0n;

	for (let i = 0; i < width; i++) {
		value = (value << 8n) | BigInt(bytes[at + 1 + i]);
	}

	return { value, at: at + 1 + width };
};

const readInt = (
	bytes: Uint8Array,
	at: number,
): { value: bigint; at: number } | null => {
	const raw = readUint(bytes, at);

	if (!raw) return null;

	if ((raw.value & 1n) === 1n) {
		return { value: ~(raw.value >> 1n), at: raw.at };
	}

	return { value: raw.value >> 1n, at: raw.at };
};

const floatFromGob = (bits: bigint): number => {
	let reversed = 0n;
	let rest = bits;

	for (let i = 0; i < 8; i++) {
		reversed = (reversed << 8n) | (rest & 0xffn);
		rest >>= 8n;
	}

	const view = new DataView(new ArrayBuffer(8));
	view.setBigUint64(0, reversed, false);

	return view.getFloat64(0, false);
};

const readMessage = (
	bytes: Uint8Array,
	at: number,
): { body: Uint8Array; at: number } | null => {
	const length = readUint(bytes, at);

	if (!length) return null;

	const count = Number(length.value);

	if (!Number.isSafeInteger(count) || length.at + count > bytes.length) {
		return null;
	}

	return {
		body: bytes.subarray(length.at, length.at + count),
		at: length.at + count,
	};
};

export const gobLayer = (bytes: Uint8Array): GobLayer | null => {
	let at = 0;
	let found: GobLayer | null = null;

	while (at < bytes.length) {
		const message = readMessage(bytes, at);

		if (!message || message.body.length === 0) return found;

		at = message.at;

		const id = readInt(message.body, 0);

		if (!id) return found;

		if (id.value < 0n) continue;

		const delta = readUint(message.body, id.at);

		if (!delta) return found;

		if (id.value === 5n) {
			const count = readUint(message.body, delta.at);

			if (!count) return found;

			const n = Number(count.value);

			if (!Number.isSafeInteger(n) || count.at + n > message.body.length) {
				return found;
			}

			found = {
				kind: "bytes",
				bytes: message.body.subarray(count.at, count.at + n),
			};
			continue;
		}

		if (id.value >= 64n) {
			const count = readUint(message.body, delta.at);

			if (!count) return found;

			const n = Number(count.value);

			if (!Number.isSafeInteger(n)) return found;

			const floats: number[] = [];
			let cursor = count.at;

			for (let i = 0; i < n; i++) {
				const bits = readUint(message.body, cursor);

				if (!bits) return found;

				floats.push(floatFromGob(bits.value));
				cursor = bits.at;
			}

			found = { kind: "floats", floats };
		}
	}

	return found;
};
