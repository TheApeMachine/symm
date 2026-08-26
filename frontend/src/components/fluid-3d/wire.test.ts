import { describe, expect, it } from "vitest";
import { decodeFields, decodeParticles, decodePhase } from "./wire";

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
		expect(particles.amplitudeScale).toBe(9);
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

describe("decodePhase", () => {
	it("reads the reading, downsample oscillators, and spectral modes from one SPH1 slab", () => {
		const oscillatorCount = 2;
		const modeCount = 2;
		const modesOffset = 64 + oscillatorCount * 8;
		const byteLength =
			modesOffset + modeCount * 16;
		const { buffer, view } = header("SPH1", byteLength);
		view.setFloat32(16, 1.5, true);
		view.setFloat32(20, 2.5, true);
		view.setFloat32(24, 3.5, true);
		view.setFloat32(28, 4.5, true);
		view.setFloat32(32, 5.5, true);
		view.setFloat32(36, 0.75, true);
		view.setUint32(40, oscillatorCount, true);
		view.setUint32(44, modeCount, true);
		view.setUint32(48, byteLength, true);
		const phases = new Float32Array(buffer, 64, oscillatorCount);
		const omegas = new Float32Array(buffer, 64 + oscillatorCount * 4, oscillatorCount);
		const modeOmega = new Float32Array(buffer, modesOffset, modeCount);
		const modeReal = new Float32Array(buffer, modesOffset + modeCount * 4, modeCount);
		const modeImag = new Float32Array(buffer, modesOffset + modeCount * 8, modeCount);
		const modeLinewidth = new Float32Array(buffer, modesOffset + modeCount * 12, modeCount);
		phases.set([0.1, 0.2]);
		omegas.set([1, 2]);
		modeOmega.set([-2, 2]);
		modeReal.set([0.5, -0.5]);
		modeImag.set([0.25, -0.25]);
		modeLinewidth.set([0.3, 0.3]);

		const phase = decodePhase(new Uint8Array(buffer));

		expect(phase.sequence).toBe(7n);
		expect(phase.reading).toEqual({
			divergence: 1.5,
			guidanceSpeed: 2.5,
			coherenceMag2: 3.5,
			pressureGradNorm: 4.5,
			viscosityProxy: 5.5,
			kuramotoR: 0.75,
		});
		expect(phase.oscillators).toHaveLength(2);
		expect(phase.oscillators[0].phase).toBeCloseTo(0.1, 5);
		expect(phase.oscillators[0].omega).toBe(1);
		expect(phase.oscillators[1].phase).toBeCloseTo(0.2, 5);
		expect(phase.oscillators[1].omega).toBe(2);
		expect(phase.modes).toHaveLength(2);
		expect(phase.modes[0].omega).toBe(-2);
		expect(phase.modes[0].real).toBe(0.5);
		expect(phase.modes[0].imaginary).toBe(0.25);
		expect(phase.modes[0].linewidth).toBeCloseTo(0.3, 5);
		expect(phase.modes[1].omega).toBe(2);
		expect(phase.modes[1].real).toBe(-0.5);
		expect(phase.modes[1].imaginary).toBe(-0.25);
		expect(phase.modes[1].linewidth).toBeCloseTo(0.3, 5);
	});
});
