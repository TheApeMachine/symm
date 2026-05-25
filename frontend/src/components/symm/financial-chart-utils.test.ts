import { describe, expect, it } from "vitest";

import { priceLabelDecimals } from "#/components/symm/financial-chart-utils";

describe("priceLabelDecimals", () => {
	it("uses finer precision for narrow live price spans", () => {
		expect(priceLabelDecimals(0.05)).toBe(4);
		expect(priceLabelDecimals(5)).toBe(3);
		expect(priceLabelDecimals(500)).toBe(1);
		expect(priceLabelDecimals(5000)).toBe(0);
	});
});
