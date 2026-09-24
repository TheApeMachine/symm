import { describe, expect, it } from "vitest";
import { calculateEdgePath, type ObstacleRect } from "./connection-path-math";

/*
	The corridor scan files obstacles into bands so it does not walk all of
	them per step. An obstacle is only found if it was filed into every band
	it covers, so the cases that matter are the ones sitting on a boundary.
*/
describe("routing around a banded obstacle", () => {
	const from = { x: 0, y: 0 };
	const to = { x: 800, y: 0 };

	const pathWith = (obstacles: ObstacleRect[], y: number) =>
		calculateEdgePath(
			"orthogonal",
			{ ...from, y },
			{ ...to, y },
			obstacles,
			obstacles,
		);

	it("goes straight when nothing is in the way", () => {
		const clear = pathWith([], 0);

		expect(clear).toContain("M 0 0");
	});

	// 256 is the band width, so these straddle a boundary in both axes.
	for (const y of [0, 255, 256, 257, 511, 512, 1024, -256, -1]) {
		it(`finds an obstacle blocking the run at y=${y}`, () => {
			const blocker: ObstacleRect = {
				left: 300,
				right: 500,
				top: y - 100,
				bottom: y + 100,
			};

			const blocked = pathWith([blocker], y);
			const clear = pathWith([], y);

			// It cannot have routed the same way through something solid.
			expect(blocked).not.toEqual(clear);
		});
	}

	it("finds an obstacle taller than a band", () => {
		const tall: ObstacleRect = {
			left: 300,
			right: 500,
			top: -2000,
			bottom: 2000,
		};

		expect(pathWith([tall], 0)).not.toEqual(pathWith([], 0));
	});
});
