import { describe, expect, it } from "vitest";
import {
	ingestManifoldWire,
	isManifoldSnapshot,
	type ManifoldPushBridge,
} from "#/components/charts/manifold/manifold-push-bridge";

describe("isManifoldSnapshot", () => {
	it("accepts manifold field snapshots", () => {
		expect(
			isManifoldSnapshot({
				type: "manifold",
				ts: "2024-01-01T00:00:00Z",
				grid: { x: 4, y: 1, z: 4, spacing: 1 },
				rho: [
					[0, 1],
					[2, 3],
				],
				reading: {
					pressure_grad_x: 0,
					pressure_grad_y: 0,
					pressure_grad_z: 0,
					pressure_grad_norm: 1,
					divergence: 0,
					coherence_mag2: 0.5,
					guidance_speed: 0.2,
					viscosity_proxy: 0.1,
				},
				carriers: [],
			}),
		).toBe(true);
	});

	it("rejects fluid snapshots", () => {
		expect(
			isManifoldSnapshot({
				type: "fluid",
				ts: "2024-01-01T00:00:00Z",
				symbol_count: 1,
				symbols: [],
			}),
		).toBe(false);
	});
});

describe("manifold push bridge", () => {
	it("buffers frames until attach", () => {
		const bridge: ManifoldPushBridge = {
			push: () => {},
			ready: false,
			pending: null,
		};

		const frame = {
			type: "manifold" as const,
			ts: "2024-01-01T00:00:00Z",
			grid: { x: 2, y: 1, z: 2, spacing: 1 },
			rho: [[0, 1]],
			reading: {
				pressure_grad_x: 0,
				pressure_grad_y: 0,
				pressure_grad_z: 0,
				pressure_grad_norm: 0,
				divergence: 0,
				coherence_mag2: 0,
				guidance_speed: 0,
				viscosity_proxy: 0,
			},
			carriers: [],
		};

		ingestManifoldWire(bridge, frame);

		expect(bridge.pending).toEqual(frame);
	});
});
