import { describe, expect, it } from "vitest";
import { layerCellsFromState } from "#/components/terminal/xray-layers";

describe("xray layer projection", () => {
	it("keeps native 16-cell resonance layers unchanged", () => {
		const state = Array.from({ length: 16 }, (_, index) => index / 16);

		expect(layerCellsFromState(state)).toEqual(state);
	});

	it("projects shorter backend vectors into the fixed mockup strip", () => {
		const cells = layerCellsFromState([-1, 0, 1], 5);

		expect(cells).toEqual([-1, -0.5, 0, 0.5, 1]);
	});
});
