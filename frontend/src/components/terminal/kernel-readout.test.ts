import { describe, expect, it } from "vitest";
import { kernelReadout, kernelStatus } from "./kernel-readout";

describe("kernel readout", () => {
	it("treats confidence-bearing backend measurements as measured when status is absent", () => {
		const frame = {
			origin: "fluid",
			scope: "SPX/USD",
			output: {
				confidence: 0.97,
				surprise: 0,
			},
		};

		expect(kernelStatus(frame)).toBe("measured");
		expect(kernelReadout(frame).status).toBe("measured");
	});

	it("keeps status unknown when no backend evidence is present", () => {
		expect(
			kernelStatus({
				origin: "fluid",
				scope: "stream",
				output: {},
			}),
		).toBe("unknown");
	});
});
