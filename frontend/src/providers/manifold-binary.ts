/*
manifold-binary decodes backend-composited SMF1 display frames. Scalar lattice
frames are ignored so the frontend cannot synthesize pilot-wave imagery.
*/

export type DisplayFrame = {
	kind: "display";
	symbol: string;
	width: number;
	height: number;
	rgba: Uint8ClampedArray;
};

let display: DisplayFrame | null = null;

const view = (buffer: ArrayBuffer) => new DataView(buffer);

/*
parseManifoldBinary decodes one backend GPU RGBA frame. It returns null for
legacy scalar planes and malformed frames so they cannot replace the picture.
*/
export const parseManifoldBinary = (
	buffer: ArrayBuffer,
): DisplayFrame | null => {
	if (buffer.byteLength < 26) {
		return null;
	}

	const data = view(buffer);

	if (
		data.getUint8(0) !== 0x53 ||
		data.getUint8(1) !== 0x4d ||
		data.getUint8(2) !== 0x46 ||
		data.getUint8(3) !== 0x31 ||
		data.getUint8(4) !== 5
	) {
		return null;
	}

	const width = data.getUint16(5, true);
	const height = data.getUint16(7, true);
	const symbolLen = data.getUint8(25);

	if (width < 1 || height < 1 || buffer.byteLength < 26 + symbolLen) {
		return null;
	}

	const symbol = new TextDecoder().decode(
		new Uint8Array(buffer, 26, symbolLen),
	);
	const offset = 26 + symbolLen;
	const count = width * height;

	if (buffer.byteLength < offset + count * 4) {
		return null;
	}

	return {
		kind: "display",
		symbol,
		width,
		height,
		// Copy out of the websocket buffer because the worker reuses transferables.
		rgba: new Uint8ClampedArray(buffer.slice(offset, offset + count * 4)),
	};
};

export const retainManifoldBinary = (buffer: ArrayBuffer): "display" | null => {
	const frame = parseManifoldBinary(buffer);

	if (frame === null) {
		return null;
	}

	display = frame;
	return "display";
};

export const latestDisplay = (): DisplayFrame | null => display;

export const clearManifoldBinary = () => {
	display = null;
};
