/*
Layer payloads are JSON bytes on a Value. A tick is read by several panels,
and each read used to allocate a TextDecoder and parse the same bytes again.
The cache is keyed by the byte array the decoder handed out, which stays put
for the life of that frame.
*/

const textDecoder = new TextDecoder();
const textEncoder = new TextEncoder();

const decoded = new WeakMap<Uint8Array, unknown>();
const invalid = new WeakSet<Uint8Array>();

export const decodeText = (bytes: Uint8Array): string =>
	textDecoder.decode(bytes);

/*
decodeJsonBytes parses one layer and remembers the value. A later read of the
same bytes returns that value, so callers must treat object payloads as
read-only.
*/
export const decodeJsonBytes = (bytes: Uint8Array): unknown => {
	if (invalid.has(bytes)) {
		throw new SyntaxError("invalid json layer");
	}

	if (decoded.has(bytes)) {
		return decoded.get(bytes);
	}

	try {
		const value = JSON.parse(textDecoder.decode(bytes));
		decoded.set(bytes, value);
		return value;
	} catch (error) {
		invalid.add(bytes);
		throw error;
	}
};

export const tryDecodeJsonBytes = (bytes: Uint8Array): unknown => {
	try {
		return decodeJsonBytes(bytes);
	} catch {
		return undefined;
	}
};

/*
encodeJsonBytes writes a payload that was split out of a larger layer. The
bytes are not cached: JSON.stringify drops NaN and Infinity, and the next
decode has to see that same JSON.
*/
export const encodeJsonBytes = (value: unknown): Uint8Array =>
	textEncoder.encode(JSON.stringify(value));
