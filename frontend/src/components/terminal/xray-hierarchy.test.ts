import { describe, expect, it } from "vitest";
import { xrayLayersFromResonance } from "#/components/terminal/xray-view";

describe("xray hierarchy retention semantics", () => {
	it("keeps prior layered state when a sparse resonance row has no layers", () => {
		const layered = xrayLayersFromResonance({
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-07-28T00:00:00Z",
			surprise: 0.25,
			layers: [{ state: [0.1, -0.2], prediction: [0, 0] }],
		});
		const sparse = xrayLayersFromResonance({
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-07-28T00:00:01Z",
			expectedReturn: 0.01,
		});

		expect(layered).toHaveLength(1);
		expect(sparse).toHaveLength(0);
	});
});
