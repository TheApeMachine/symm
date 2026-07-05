import { describe, expect, it } from "vitest";
import {
	kernelCollectionValues,
	kernelReadout,
	kernelStatus,
} from "./kernel-readout";

describe("kernel readout", () => {
	it("treats confidence-bearing backend measurements as measured when status is absent", () => {
		const frame = {
			source: "fluid",
			symbol: "SPX/USD",
			confidence: 0.97,
			surprise: 0,
		};

		expect(kernelStatus(frame)).toBe("measured");
		expect(kernelReadout(frame).status).toBe("measured");
	});

	it("keeps status unknown when no backend evidence is present", () => {
		expect(
			kernelStatus({
				source: "fluid",
				symbol: "stream",
			}),
		).toBe("unknown");
	});

	it("builds sparkline values from stored source history", () => {
		const readings = {
			measurements: {
				fluid: {
					values: () => [
						{ source: "fluid", confidence: 0.2 },
						{ source: "fluid", strength: 0.7 },
					],
				},
			},
			symbols: {},
		};

		expect(kernelCollectionValues(readings, "fluid", "stream")).toEqual([
			0.2, 0.7,
		]);
	});
});
