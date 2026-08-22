import { describe, expect, it } from "vitest";
import { finitePortraitPoint } from "./phase-portrait";

describe("finitePortraitPoint", () => {
	it("keeps a finite hydro sample", () => {
		expect(finitePortraitPoint(-0.2, 0.4)).toEqual({
			divergence: -0.2,
			pressureGradNorm: 0.4,
		});
	});

	it("rejects NaN, infinity, and non-numeric wire values", () => {
		expect(finitePortraitPoint(Number.NaN, 1)).toBeNull();
		expect(finitePortraitPoint(1, Number.POSITIVE_INFINITY)).toBeNull();
		expect(finitePortraitPoint(undefined, 1)).toBeNull();
		expect(finitePortraitPoint(1, null)).toBeNull();
	});
});
