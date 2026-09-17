import { describe, expect, it } from "vitest";
import { computeDistributionPath } from "./distribution-curve";

describe("computeDistributionPath", () => {
	it("computes SVG line and area paths without NaN", () => {
		const { linePath, areaPath, meanX, zeroX } = computeDistributionPath(
			2.5,
			3.0,
			-15,
			15,
			300,
			150,
		);

		expect(linePath).toMatch(/^M /);
		expect(linePath).not.toContain("NaN");
		expect(areaPath).toMatch(/Z$/);
		expect(areaPath).not.toContain("NaN");
		expect(meanX).toBeGreaterThan(0);
		expect(zeroX).toBeGreaterThan(0);
		expect(meanX).toBeGreaterThan(zeroX);
	});

	it("handles zero or small standard deviation gracefully", () => {
		const result = computeDistributionPath(0, 0, -10, 10, 200, 100);
		expect(result.linePath).not.toContain("NaN");
		expect(result.areaPath).not.toContain("NaN");
	});
});
