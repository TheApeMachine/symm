import { describe, expect, it } from "vitest";
import {
	CHOICE_WINDOW_MS,
	choiceWindow,
	samplesInWindow,
} from "./open-choice-trail";

describe("choice window", () => {
	it("slides by the elapsed time and keeps the same span", () => {
		const first = choiceWindow(1_000_000);
		const later = choiceWindow(1_000_000 + 30_000);

		expect(first.span).toBe(CHOICE_WINDOW_MS);
		expect(later.span).toBe(first.span);
		expect(later.from - first.from).toBe(30_000);
		expect(later.through - first.through).toBe(30_000);
	});

	it("keeps the sample that crosses the left edge", () => {
		expect(
			samplesInWindow(
				[
					{ t: 0, pnl: 1 },
					{ t: 50, pnl: 2 },
					{ t: 150, pnl: 3 },
				],
				100,
			),
		).toEqual([
			{ t: 50, pnl: 2 },
			{ t: 150, pnl: 3 },
		]);
	});
});
