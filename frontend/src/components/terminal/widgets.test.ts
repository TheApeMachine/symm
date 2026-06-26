import { describe, expect, it } from "vitest";
import { signalDetailModel } from "./widgets";

describe("signal insight detail model", () => {
	it("renders predictive coding from the live resonance measurement", () => {
		const now = new Date(2026, 5, 26, 12, 18, 55, 482).getTime();
		const model = signalDetailModel(
			{
				resonance: {
					"BTC/EUR": {
						observed_at: now - 482,
						confidence: 0.82,
						surprise: 1.28,
						output: { strength: 0.3711 },
					},
					"ETH/EUR": {
						confidence: 0.6,
						surprise: 0.4,
						output: { strength: 0.2 },
					},
				},
			},
			"prediction",
			"BTC/EUR",
			now,
		);

		expect(model.copy.name).toBe("Predictive coding");
		expect(model.activeText).toBe("2 / 2");
		expect(model.observedText).toBe("482ms / 0.5s");
		expect(model.strengthText).toBe("0.3711");
		expect(model.meters.map((meter) => meter.label)).toEqual([
			"Confidence",
			"Surprise",
			"Strength",
			"Evidence",
			"Freshness",
			"Calibration",
		]);
		expect(model.heatmap.map((cell) => cell.label)).toEqual(["BTC", "ETH"]);
	});
});
