import { describe, expect, it } from "vitest";
import { parseResonanceData, resonanceUniverseRows } from "#/lib/resonance";

describe("resonanceUniverseRows", () => {
	it("tags each store row with its symbol", () => {
		expect(
			resonanceUniverseRows({
				"BTC/USD": { energy: 1 },
				"ETH/USD": { surprise: 2 },
			}),
		).toEqual([
			{ symbol: "BTC/USD", energy: 1 },
			{ symbol: "ETH/USD", surprise: 2 },
		]);
	});
});

describe("parseResonanceData", () => {
	it("unfolds flat solver keys into vectors and layers", () => {
		const data = parseResonanceData({
			task_relative_precision: 0.42,
			supported_horizon: 3,
			forward_curve_0: 0.1,
			forward_curve_2: -0.2,
			latent_0: 1,
			latent_1: 2,
			layer_0_state_0: 0.5,
			layer_0_prediction_0: 0.4,
			layer_0_error: 0.05,
		});

		expect(data?.forwardCurve).toEqual([0.1, 0, -0.2]);
		expect(data?.latent).toEqual([1, 2]);
		expect(data?.layers[0]?.state).toEqual([0.5]);
		expect(data?.layers[0]?.prediction).toEqual([0.4]);
		expect(data?.layers[0]?.errorNorm).toBe(0.05);
		expect(data?.taskRelativePrecision).toBe(0.42);
		expect(data?.supportedHorizon).toBe(3);
	});
});