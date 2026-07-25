/*
manifold-binary decodes SMF1 uint16-scaled lattice frames into Float32 planes
for WebGL upload. Wire size stays under the old JSON decimal lattices.
*/

export type LatticeKind = "rho" | "psi" | "guidance_x" | "guidance_z";

export type LatticePlane = {
	kind: LatticeKind;
	symbol: string;
	width: number;
	height: number;
	min: number;
	max: number;
	samples: Float32Array;
};

const KIND_BY_CODE: Record<number, LatticeKind> = {
	1: "rho",
	2: "psi",
	3: "guidance_x",
	4: "guidance_z",
};

const planes: Partial<Record<LatticeKind, LatticePlane>> = {};

const view = (buffer: ArrayBuffer) => new DataView(buffer);

/*
parseManifoldBinary decodes one SMF1 lattice frame. Returns null when magic or
geometry is invalid so a bad frame cannot clear a good retained plane.
*/
export const parseManifoldBinary = (
	buffer: ArrayBuffer,
): LatticePlane | null => {
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

export const retainManifoldBinary = (buffer: ArrayBuffer): LatticeKind | null => {
	const plane = parseManifoldBinary(buffer);

	if (plane === null) {
		return null;
	}

	planes[plane.kind] = plane;
	return plane.kind;
};

export const latestLattice = (kind: LatticeKind): LatticePlane | null =>
	planes[kind] ?? null;

export const clearManifoldBinary = () => {
	for (const kind of Object.keys(planes) as LatticeKind[]) {
		delete planes[kind];
	}
};

/*
latticeMatrix materializes a plane as row-major number[][] for CPU painters
that still expect nested arrays (meta stats, phase helpers).
*/
export const latticeMatrix = (kind: LatticeKind): number[][] => {
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
withBinaryLattices overlays retained SMF1 planes onto a meta frame so painters
that still read rho/psiMag2/guidance from JSON keep working after the split.
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
