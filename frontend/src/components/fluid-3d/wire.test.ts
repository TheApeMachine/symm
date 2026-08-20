import { describe, expect, it } from "vitest";
import { decodeFields, decodeParticles } from "./wire";

const header = (magic: string, byteLength: number) => {
	const buffer = new ArrayBuffer(byteLength);
	const bytes = new Uint8Array(buffer);
	bytes.set(new TextEncoder().encode(magic));
	const view = new DataView(buffer);
	view.setUint16(4, 1, true);
	view.setUint16(6, 64, true);
	view.setBigUint64(8, 7n, true);
	return { buffer, view };
};

describe("decodeFields", () => {
	it("constructs direct typed views over one GPU-shaped field slab", () => {
		const cells = 8;
		const energyOffset = 64 + cells * 4 * Float32Array.BYTES_PER_ELEMENT;
		const waveRealOffset =
			energyOffset + cells * Float32Array.BYTES_PER_ELEMENT;
		const waveImaginaryOffset =
			waveRealOffset + cells * Float32Array.BYTES_PER_ELEMENT;
		const byteLength =
			waveImaginaryOffset + cells * Float32Array.BYTES_PER_ELEMENT;
		const { buffer, view } = header("SFF1", byteLength);
		view.setUint32(16, 2, true);
		view.setUint32(20, 2, true);
		view.setUint32(24, 2, true);
		view.setFloat32(28, 0.5, true);
		view.setFloat32(32, 1, true);
		view.setFloat32(36, 0.2, true);
		view.setFloat32(40, 0.5, true);
		view.setFloat32(44, 0.25, true);
		view.setUint32(48, energyOffset, true);
		view.setUint32(52, waveRealOffset, true);
		view.setUint32(56, waveImaginaryOffset, true);
		view.setUint32(60, byteLength, true);
		new Float32Array(buffer, 64, cells * 4)[3] = 9;

		const fields = decodeFields(new Uint8Array(buffer));

		expect(fields.sequence).toBe(7n);
		expect(fields.grid).toEqual({ x: 2, y: 2, z: 2, spacing: 0.5 });
		expect(fields.momRho.buffer).toBe(buffer);
		expect(fields.internalEnergy.buffer).toBe(buffer);
		expect(fields.momRho[3]).toBe(9);
	});
});

describe("decodeParticles", () => {
	it("keeps one interleaved view and materializes only the selected row", () => {
		const stride = 12;
		const byteLength = 64 + stride * Float32Array.BYTES_PER_ELEMENT;
		const { buffer, view } = header("SPF1", byteLength);
		view.setUint32(16, 1, true);
		view.setUint32(20, stride, true);
		view.setFloat32(24, 2, true);
		view.setFloat32(28, 3, true);
		view.setFloat32(32, 4, true);
		view.setUint32(36, byteLength, true);
		const values = new Float32Array(buffer, 64, stride);
		values.set([0.1, 0.2, 0.3, 1, 2, 3, 4, 5, 6, 7, 8, 9]);

		const particles = decodeParticles(new Uint8Array(buffer));

		expect(particles.values.buffer).toBe(buffer);
		expect(particles.particle(0)).toEqual({
			Position: { X: values[0], Y: values[1], Z: values[2] },
			Velocity: { X: 1, Y: 2, Z: 3 },
			Mass: 4,
			Heat: 5,
			Energy: 6,
			Phase: 7,
			Omega: 8,
			Amplitude: 9,
		});
	});
});
