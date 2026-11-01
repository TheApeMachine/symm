import { describe, expect, it } from "vitest";
import {
	getCanonicalRegionCounts,
	REGION_COUNT,
	sympathyAnchor,
} from "./grid-pins";

describe("sympathyAnchor", () => {
	it("keeps enter on the left, exit on the right, hold mid, oracles along the bottom", () => {
		const enter = [0, 1, 2].map((key) => sympathyAnchor(key, 1000, 800));
		const exit = [3, 4, 5].map((key) => sympathyAnchor(key, 1000, 800));
		const hold = [6, 7, 8].map((key) => sympathyAnchor(key, 1000, 800));
		const oracle = [9, 15, 22].map((key) => sympathyAnchor(key, 1000, 800));

		for (const point of enter) {
			expect(point.x).toBeLessThan(400);
		}
		for (const point of exit) {
			expect(point.x).toBeGreaterThan(600);
		}
		for (const point of hold) {
			expect(point.y).toBeGreaterThan(enter[0].y);
			expect(point.y).toBeLessThan(oracle[0].y);
		}
		for (const point of oracle) {
			expect(point.y).toBeGreaterThan(hold[0].y);
		}
		expect(oracle[0].x).toBeLessThan(oracle[1].x);
		expect(oracle[1].x).toBeLessThan(oracle[2].x);
	});
});

describe("region pin balance", () => {
	it("covers every action channel on R00–R08", () => {
		const counts = getCanonicalRegionCounts();
		for (let region = 0; region <= 8; region++) {
			expect(counts[region]).toBeGreaterThanOrEqual(1);
		}
	});

	it("envelope-backed oracle sides are wired; broker-only sides stay empty", () => {
		const counts = getCanonicalRegionCounts();

		// Volume Quote + Day Move veto are broker-only today.
		expect(counts[9]).toBe(0);
		expect(counts[10]).toBe(0);
		expect(counts[11]).toBe(0);

		for (const region of [12, 13, 14, 16, 17, 18, 19, 20, 21, 22]) {
			expect(counts[region]).toBeGreaterThanOrEqual(1);
		}

		expect(Object.keys(counts).length).toBe(REGION_COUNT);
	});
});
