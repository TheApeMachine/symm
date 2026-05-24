import { describe, expect, it } from "vitest";

import { formatFluidScalar } from "#/lib/symm/fluid-format";

describe("formatFluidScalar", () => {
	it("formats normalized activity with two decimals", () => {
		expect(formatFluidScalar(2.04)).toBe("2.04");
	});

	it("shows zero without decimals", () => {
		expect(formatFluidScalar(0)).toBe("0");
	});
});
