import { describe, expect, it } from "vitest";
import { computeSparklinePaths } from "./sparkline";

describe("computeSparklinePaths", () => {
	it("preserves excursions outside the unit interval instead of clipping them", () => {
		expect(computeSparklinePaths([-100, 0, 100]).spark).toBe(
			"0.0,29.0 75.0,16.0 150.0,3.0",
		);
		expect(computeSparklinePaths([300, 200, 100]).spark).toBe(
			"0.0,3.0 75.0,16.0 150.0,29.0",
		);
	});
	it("keeps empty input empty and places constant data at the vertical midpoint", () => {
		expect(computeSparklinePaths([]).spark).toBe("");
		expect(computeSparklinePaths([]).area).toBe("");
		expect(computeSparklinePaths([7, 7]).spark).toBe("0.0,16.0 150.0,16.0");
		expect(computeSparklinePaths([7]).spark).toBe("75.0,16.0");
	});
});
