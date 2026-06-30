import { describe, expect, it } from "vitest";
import { kernelsForFocus } from "#/components/terminal/kernels";

describe("terminal kernels", () => {
	it("does not borrow another symbol for a concrete focus", () => {
		const kernels = kernelsForFocus(
			{
				fluid: {
					"ETH/USD": {
						output: { confidence: 0.8, surprise: 0.4 },
					},
				},
			},
			"BTC/USD",
		);

		expect(kernels).toEqual([]);
	});

	it("uses a live symbol only in stream mode", () => {
		const kernels = kernelsForFocus(
			{
				fluid: {
					"ETH/USD": {
						output: { confidence: 0.8, surprise: 0.4 },
					},
				},
			},
			"stream",
		);

		expect(kernels).toEqual([
			{ source: "fluid", confidencePercent: 80, surprisePercent: 40 },
		]);
	});
});
