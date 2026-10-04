import { describe, expect, it } from "vitest";
import { pilotCurrentGlyphs } from "./current";
import { FluidParticleFrame } from "./wire";

const particles = (guidance: Float32Array | null) =>
	new FluidParticleFrame(
		1n,
		2,
		new Float32Array([0.25, 0.5, 0.5, 0.75, 0.5, 0.5]),
		new Float32Array([99, 0, 0, 99, 0, 0]),
		new Float32Array(2),
		new Float32Array(2),
		new Float32Array(2),
		new Float32Array(2),
		new Float32Array(2),
		new Float32Array(2),
		guidance,
	);

describe("pilotCurrentGlyphs", () => {
	it("distinguishes missing telemetry from actual zero guidance", () => {
		expect(pilotCurrentGlyphs(particles(null), 1 / 64)).toEqual({
			vertices: new Float32Array(0),
			peak: null,
		});
		expect(pilotCurrentGlyphs(particles(new Float32Array(6)), 1 / 64)).toEqual({
			vertices: new Float32Array(0),
			peak: 0,
		});
	});
	it("draws the solver vectors at their particle positions, not total velocity", () => {
		const frame = particles(new Float32Array([0, 2, 0, 0, 0, -1]));
		const { vertices, peak } = pilotCurrentGlyphs(frame, 1 / 64);
		expect(peak).toBe(2);
		expect(vertices.length).toBe(60);
		expect(vertices[5] - vertices[0]).toBe(0);
		expect(vertices[6] - vertices[1]).toBe(2 / 64);
		expect(vertices[7] - vertices[2]).toBe(0);
		expect((vertices[0] + vertices[5]) / 2).toBe(0.25);
		expect(vertices[37] - vertices[32]).toBe(-1 / 64);
		expect(Array.from(frame.pilotVel!)).toEqual([0, 2, 0, 0, 0, -1]);
	});
	it("retains tiny backend velocities in the readout while scaling only arrow geometry", () => {
		const tiny = particles(new Float32Array([0, 2e-12, 0, 0, 0, -1e-12]));
		const result = pilotCurrentGlyphs(tiny, 1 / 64);
		expect(result.peak).toBe(tiny.pilotVel![1]);
		expect(result.vertices[6] - result.vertices[1]).toBe(2 / 64);
	});
	it("rejects mismatched telemetry instead of inventing missing vectors", () => {
		expect(() =>
			pilotCurrentGlyphs(particles(new Float32Array(3)), 1 / 64),
		).toThrow("particle count");
	});
});
