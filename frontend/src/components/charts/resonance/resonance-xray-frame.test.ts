import { describe, expect, it } from "vitest";
import {
	latentPoint3D,
	parseResonanceXRayFrame,
	recentHeatmapTimeRange,
} from "#/components/charts/resonance/resonance-xray-frame";

describe("latentPoint3D", () => {
	it("maps a 3D latent state into phase space", () => {
		const point = latentPoint3D([
			{ state: [1, 2, 3, 4], prediction: [], errorNorm: 0 },
			{ state: [0.2, -0.4, 0.7], prediction: [], errorNorm: 0 },
		]);

		expect(point).toEqual({ x: 0.2, y: -0.4, z: 0.7 });
	});

	it("embeds 2D latent states in the xz plane", () => {
		const point = latentPoint3D([
			{ state: [0.5, -0.25], prediction: [], errorNorm: 0 },
		]);

		expect(point).toEqual({ x: 0.5, y: 0, z: -0.25 });
	});
});

describe("recentHeatmapTimeRange", () => {
	it("anchors the visible window on the latest columns", () => {
		const range = recentHeatmapTimeRange(120, 48);

		expect(range.start).toBe(72);
		expect(range.end).toBe(120);
	});
});

describe("parseResonanceXRayFrame", () => {
	it("accepts resonance wire frames with layer snapshots", () => {
		const frame = parseResonanceXRayFrame({
			type: "resonance",
			symbol: "PF_XBTUSD",
			surprise: 0.12,
			energy: 0.4,
			confidence: 0.88,
			category: "laminar_resonance",
			layers: [
				{
					state: [1, 2, 3, 4],
					prediction: [0.9, 1.8, 2.9, 3.8],
					error_norm: 0.05,
				},
				{
					state: [0.1, 0.2, 0.3],
					prediction: [0.08, 0.18, 0.28],
					error_norm: 0.02,
				},
			],
		});

		expect(frame?.symbol).toBe("PF_XBTUSD");
		expect(frame?.layers).toHaveLength(2);
	});
});
