/*
manifold-binary decodes SMF1 frames. Display frames are backend-composited
RGBA8 textures the fluid canvas blits; legacy uint16 planes remain decodable
for older fixtures.
*/

export type LatticeKind =
	| "rho"
	| "psi"
	| "guidance_x"
	| "guidance_z"
	| "display";

export type LatticePlane = {
	kind: Exclude<LatticeKind, "display">;
	symbol: string;
	width: number;
	height: number;
	min: number;
	max: number;
	samples: Float32Array;
};

export type DisplayFrame = {
	kind: "display";
	symbol: string;
	width: number;
	height: number;
	rgba: Uint8ClampedArray;
};

const KIND_BY_CODE: Record<number, LatticeKind> = {
	1: "rho",
	2: "psi",
	3: "guidance_x",
	4: "guidance_z",
	5: "display",
};

const planes: Partial<
	Record<Exclude<LatticeKind, "display">, LatticePlane>
> = {};
let display: DisplayFrame | null = null;

const view = (buffer: ArrayBuffer) => new DataView(buffer);

/*
parseManifoldBinary decodes one SMF1 frame. Returns null when magic or
geometry is invalid so a bad frame cannot clear a good retained picture.
*/
export const parseManifoldBinary = (
	buffer: ArrayBuffer,
): LatticePlane | DisplayFrame | null => {
	if (buffer.byteLength < 26) {
		return null;
	}

	const data = view(buffer);

	if (
		data.getUint8(0) !== 0x53 ||
		data.getUint8(1) !== 0x4d ||
		data.getUint8(2) !== 0x46 ||
		data.getUint8(3) !== 0x31
	) {
		return null;
	}

	const kind = KIND_BY_CODE[data.getUint8(4)];

	if (kind === undefined) {
		return null;
	}

	const width = data.getUint16(5, true);
	const height = data.getUint16(7, true);
	const min = data.getFloat32(9, true);
	const max = data.getFloat32(13, true);
	const symbolLen = data.getUint8(25);

	if (width < 1 || height < 1 || buffer.byteLength < 26 + symbolLen) {
		return null;
	}

	const symbol = new TextDecoder().decode(
		new Uint8Array(buffer, 26, symbolLen),
	);
	const offset = 26 + symbolLen;
	const count = width * height;

	if (kind === "display") {
		if (buffer.byteLength < offset + count * 4) {
			return null;
		}

		return {
			kind: "display",
			symbol,
			width,
			height,
			// Copy out of the websocket buffer — the worker reuses transferables.
			rgba: new Uint8ClampedArray(
				buffer.slice(offset, offset + count * 4),
			),
		};
	}

	if (buffer.byteLength < offset + count * 2) {
		return null;
	}

	const span = max - min;
	const samples = new Float32Array(count);

	for (let index = 0; index < count; index += 1) {
		const coded = data.getUint16(offset + index * 2, true);
		samples[index] = span <= 0 ? min : min + (coded / 65535) * span;
	}

	return { kind, symbol, width, height, min, max, samples };
};

export const retainManifoldBinary = (
	buffer: ArrayBuffer,
): LatticeKind | null => {
	const plane = parseManifoldBinary(buffer);

	if (plane === null) {
		return null;
	}

	if (plane.kind === "display") {
		display = plane;
		return "display";
	}

	planes[plane.kind] = plane;
	return plane.kind;
};

export const latestDisplay = (): DisplayFrame | null => display;

export const latestLattice = (
	kind: Exclude<LatticeKind, "display">,
): LatticePlane | null => planes[kind] ?? null;

export const clearManifoldBinary = () => {
	for (const kind of Object.keys(planes) as Array<
		Exclude<LatticeKind, "display">
	>) {
		delete planes[kind];
	}

	display = null;
};

/*
latticeMatrix materializes a plane as row-major number[][] for CPU painters
that still expect nested arrays (meta stats, phase helpers).
*/
export const latticeMatrix = (
	kind: Exclude<LatticeKind, "display">,
): number[][] => {
	const plane = planes[kind];

	if (plane === undefined) {
		return [];
	}

	const rows: number[][] = Array.from({ length: plane.height }, () =>
		Array.from({ length: plane.width }, () => 0),
	);

	for (let row = 0; row < plane.height; row += 1) {
		const line = rows[row];

		if (line === undefined) {
			continue;
		}

		for (let column = 0; column < plane.width; column += 1) {
			line[column] = plane.samples[row * plane.width + column] ?? 0;
		}
	}

	return rows;
};

/*
withBinaryLattices overlays retained scalar planes onto a meta frame when
present. Display-only publishes leave the frame scalars untouched.
*/
export const withBinaryLattices = <T extends Record<string, unknown> | null>(
	frame: T,
): T => {
	if (frame === null) {
		return frame;
	}

	const rho = latticeMatrix("rho");
	const psiMag2 = latticeMatrix("psi");

	if (rho.length === 0 && psiMag2.length === 0) {
		return frame;
	}

	return {
		...frame,
		rho,
		psiMag2,
		guidanceVelX: latticeMatrix("guidance_x"),
		guidanceVelZ: latticeMatrix("guidance_z"),
	};
};
