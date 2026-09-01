import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { ManifoldFrameT } from "#/providers/telemetry/telemetry/manifold-frame";
import { ManifoldReadingT } from "#/providers/telemetry/telemetry/manifold-reading";
import { WaveModeT } from "#/providers/telemetry/telemetry/wave-mode";
import { decodeManifold } from "./wire";

const encode = (frame: ManifoldFrameT): Uint8Array => {
	const builder = new flatbuffers.Builder(1024);
	const offset = frame.pack(builder);
	const envelope = Envelope.createEnvelope(
		builder,
		BigInt(0),
		Frame.ManifoldFrame,
		offset,
	);
	Envelope.finishEnvelopeBuffer(builder, envelope);
	return builder.asUint8Array();
};

describe("decodeManifold", () => {
	it("decodes one particle's fields, grid, and wave modes from a real ManifoldFrame", () => {
		const frame = new ManifoldFrameT(
			BigInt(7),
			BigInt(1),
			[10n],
			[11n],
			[12n],
			[13n],
			[0.1],
			[1],
			[6],
			[4],
			[5],
			[9],
			[0.1, 0.2, 0.3],
			[1, 2, 3],
			[false],
			[false],
			new ManifoldReadingT(1.5, 2.5, 3.5, 4.5, 5.5, 0.75),
			2,
			2,
			2,
			0.5,
			[9],
			[0.5, 0.75],
			[0.25],
			[-0.25],
			1,
			0.2,
			0.5,
			0.25,
			[new WaveModeT(-2, 0.5, 0.25, 0.3), new WaveModeT(2, -0.5, -0.25, 0.3)],
		);

		const decoded = decodeManifold(encode(frame));

		expect(decoded.fields.sequence).toBe(7n);
		expect(decoded.fields.grid).toEqual({ x: 2, y: 2, z: 2, spacing: 0.5 });
		expect(Array.from(decoded.fields.momRho)).toEqual([9]);
		expect(Array.from(decoded.fields.internalEnergy)).toEqual([0.5, 0.75]);
		expect(Array.from(decoded.fields.waveReal)).toEqual([0.25]);
		expect(Array.from(decoded.fields.waveImaginary)).toEqual([-0.25]);
		expect(decoded.fields.densityScale).toBe(1);
		expect(decoded.fields.momentumScale).toBeCloseTo(0.2, 5);
		expect(decoded.fields.energyScale).toBe(0.5);
		expect(decoded.fields.waveScale).toBe(0.25);

		expect(decoded.particles.count).toBe(1);
		expect(decoded.particles.particle(0)).toEqual({
			Position: {
				X: expect.closeTo(0.1, 5),
				Y: expect.closeTo(0.2, 5),
				Z: expect.closeTo(0.3, 5),
			},
			Velocity: { X: 1, Y: 2, Z: 3 },
			Mass: 4,
			Heat: 5,
			Energy: 6,
			Phase: expect.closeTo(0.1, 5),
			Omega: 1,
			Amplitude: 9,
		});

		expect(decoded.phase.reading).toEqual({
			divergence: 1.5,
			guidanceSpeed: 2.5,
			coherenceMag2: 3.5,
			pressureGradNorm: 4.5,
			viscosityProxy: 5.5,
			kuramotoR: 0.75,
		});
		expect(decoded.phase.oscillators).toHaveLength(1);
		expect(decoded.phase.oscillators[0]).toEqual({
			phase: expect.closeTo(0.1, 5),
			omega: 1,
			amplitude: 9,
			side: "bid",
		});
		expect(decoded.phase.modes).toHaveLength(2);
		expect(decoded.phase.modes[0]).toEqual({
			omega: -2,
			real: 0.5,
			imaginary: 0.25,
			linewidth: expect.closeTo(0.3, 5),
		});
		expect(decoded.phase.modes[1]).toEqual({
			omega: 2,
			real: -0.5,
			imaginary: -0.25,
			linewidth: expect.closeTo(0.3, 5),
		});
	});
});
