import { describe, expect, it } from "vitest";

import { formatFluidScalar, headerFieldMetrics } from "#/lib/symm/fluid-format";

describe("formatFluidScalar", () => {
	it("formats normalized activity with two decimals", () => {
		expect(formatFluidScalar(2.04)).toBe("2.04");
	});

	it("shows zero without decimals", () => {
		expect(formatFluidScalar(0)).toBe("0");
	});

	it("shows small non-zero values with four decimals", () => {
		expect(formatFluidScalar(0.0034)).toBe("0.0034");
	});
});

describe("headerFieldMetrics", () => {
	it("falls back to symbol medians when aggregate activity is zero", () => {
		const metrics = headerFieldMetrics(
			{ re: 0, vort: 0, div: 0, turb: 0, visc: 0 },
			[
				{
					symbol: "BTC/EUR",
					change_pct: 0.1,
					vol: 10,
					re: 0.12,
					vort: 0.04,
					div: -0.2,
					turb: 0.01,
					visc: 1,
				},
				{
					symbol: "ETH/EUR",
					change_pct: 0.2,
					vol: 8,
					re: 0.08,
					vort: 0.06,
					div: -0.4,
					turb: 0.02,
					visc: 1,
				},
			],
		);

		expect(metrics.re).toBeCloseTo(0.1);
		expect(metrics.vort).toBeCloseTo(0.05);
		expect(metrics.div).toBeCloseTo(-0.3);
		expect(metrics.turb).toBeCloseTo(0.015);
	});

	it("uses active symbol medians when sparse activity leaves aggregate at zero", () => {
		const metrics = headerFieldMetrics(
			{ re: 0, vort: 0, div: 0, turb: 0, visc: 0 },
			[
				{
					symbol: "BTC/EUR",
					change_pct: 0,
					vol: 10,
					re: 0,
					vort: 0,
					div: 0,
					turb: 0,
					visc: 1,
				},
				{
					symbol: "ETH/EUR",
					change_pct: 0.2,
					vol: 8,
					re: 0.08,
					vort: 0.06,
					div: -0.4,
					turb: 0.02,
					visc: 1,
				},
			],
		);

		expect(metrics.re).toBeCloseTo(0.08);
		expect(metrics.vort).toBeCloseTo(0.06);
		expect(metrics.div).toBeCloseTo(-0.4);
		expect(metrics.turb).toBeCloseTo(0.02);
	});
});
