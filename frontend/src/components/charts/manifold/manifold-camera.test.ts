import { describe, expect, it } from "vitest";

import {
	manifoldCameraFrame,
	manifoldHeightExtent,
	normalizeCarrierWeights,
} from "#/components/charts/manifold/manifold-camera";

describe("manifoldCameraFrame", () => {
	it("centers the orbit on the grid midpoint", () => {
		const frame = manifoldCameraFrame(32, 16, 1);

		expect(frame.centerX).toBe(3.875);
		expect(frame.centerZ).toBe(3.75);
		expect(frame.orbit).toBeGreaterThan(0);
	});
});

describe("manifoldHeightExtent", () => {
	it("adds padding when the surface is flat", () => {
		expect(manifoldHeightExtent([[0.5, 0.5]])).toEqual({
			min: 0.45,
			max: 0.55,
		});
	});
});

describe("normalizeCarrierWeights", () => {
	it("scales carrier weights relative to the strongest carrier", () => {
		const weights = normalizeCarrierWeights([
			{ role: "symbol", symbol: "BTC/USD", amplitude: 0.2, heat: 0 },
			{ role: "symbol", symbol: "ETH/USD", amplitude: 0.8, heat: 0 },
		]);

		expect(weights.get("BTC/USD")).toBe(0.25);
		expect(weights.get("ETH/USD")).toBe(1);
	});
});
