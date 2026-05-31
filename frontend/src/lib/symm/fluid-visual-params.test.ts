import { describe, expect, it } from "vitest";

import {
	defaultFluidVisualParams,
	mergeFluidVisualParams,
} from "#/lib/symm/fluid-visual-params";

describe("fluid visual params", () => {
	it("merges partial stored values and clamps to spec bounds", () => {
		const merged = mergeFluidVisualParams({
			opacity: 2,
			lightingFactor: -1,
			yMin: -0.5,
			yMax: 0.4,
		});

		expect(merged.opacity).toBe(1);
		expect(merged.lightingFactor).toBe(0);
		expect(merged.yMin).toBe(-0.5);
		expect(merged.yMax).toBe(0.4);
	});

	it("rejects collapsed height ranges", () => {
		const merged = mergeFluidVisualParams({
			yMin: 0,
			yMax: 0,
		});

		expect(merged).toEqual(defaultFluidVisualParams());
	});
});
