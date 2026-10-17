import { describe, expect, it } from "vitest";
import { sympathyAnchor } from "./grid-pins";

describe("sympathyAnchor", () => {
	it("keeps enter on the left, exit on the right, and oracles along the bottom", () => {
		const enter = [0, 1, 2].map((key) => sympathyAnchor(key, 1000, 800));
		const exit = [3, 4, 5].map((key) => sympathyAnchor(key, 1000, 800));
		const oracle = [6, 10, 14].map((key) => sympathyAnchor(key, 1000, 800));

		for (const point of enter) {
			expect(point.x).toBeLessThan(400);
		}
		for (const point of exit) {
			expect(point.x).toBeGreaterThan(600);
		}
		for (const point of oracle) {
			expect(point.y).toBeGreaterThan(enter[0].y);
			expect(point.y).toBeGreaterThan(exit[0].y);
		}
		expect(oracle[0].x).toBeLessThan(oracle[1].x);
		expect(oracle[1].x).toBeLessThan(oracle[2].x);
	});
});
