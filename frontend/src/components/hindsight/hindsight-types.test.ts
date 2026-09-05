import { describe, expect, it } from "vitest";
import {
	compareHindsightRef,
	orderHindsightRefs,
	sameHindsightRef,
} from "./hindsight-types";

describe("HindsightRef identity", () => {
	it("treats two ordinals within one capture as distinct causal points", () => {
		expect(
			sameHindsightRef(
				{ sequence: 100, ordinal: 0 },
				{ sequence: 100, ordinal: 1 },
			),
		).toBe(false);
		expect(
			sameHindsightRef(
				{ sequence: 100, ordinal: 0 },
				{ sequence: 100, ordinal: 0 },
			),
		).toBe(true);
	});

	it("orders by sequence first and ordinal second", () => {
		expect(
			compareHindsightRef(
				{ sequence: 99, ordinal: 9 },
				{ sequence: 100, ordinal: 0 },
			),
		).toBeLessThan(0);
		expect(
			compareHindsightRef(
				{ sequence: 100, ordinal: 0 },
				{ sequence: 100, ordinal: 1 },
			),
		).toBeLessThan(0);
		expect(
			compareHindsightRef(
				{ sequence: 100, ordinal: 1 },
				{ sequence: 100, ordinal: 1 },
			),
		).toBe(0);
	});

	it("never deduplicates 100:0 and 100:1 into one sequence", () => {
		const ordered = orderHindsightRefs([
			{ sequence: 100, ordinal: 1 },
			{ sequence: 100, ordinal: 0 },
			{ sequence: 99, ordinal: 0 },
		]);

		expect(ordered).toEqual([
			{ sequence: 99, ordinal: 0 },
			{ sequence: 100, ordinal: 0 },
			{ sequence: 100, ordinal: 1 },
		]);
	});

	it("deduplicates the identical full reference but keeps distinct ordinals", () => {
		const ordered = orderHindsightRefs([
			{ sequence: 100, ordinal: 1 },
			{ sequence: 100, ordinal: 1 },
			{ sequence: 100, ordinal: 0 },
		]);

		expect(ordered).toEqual([
			{ sequence: 100, ordinal: 0 },
			{ sequence: 100, ordinal: 1 },
		]);
	});
});
