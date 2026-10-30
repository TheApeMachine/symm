import { describe, expect, it } from "vitest";
import {
	CHOICE_SAMPLE_MS,
	CHOICE_TRAIL_LENGTH,
	CHOICE_WINDOW_MS,
	choiceWindow,
	indexMean,
	linePath,
	noteOpenChoice,
	openChoiceTrail,
	resetOpenChoices,
	samplesInWindow,
	stepPath,
} from "./open-choice-trail";

describe("open choice trail", () => {
	it("keeps sampling on a cadence even when return is flat", () => {
		resetOpenChoices();
		const t0 = 1_000_000;
		noteOpenChoice("doge", -1.5, t0);
		noteOpenChoice("doge", -1.5, t0 + CHOICE_SAMPLE_MS - 1);
		expect(openChoiceTrail("doge")).toHaveLength(1);

		noteOpenChoice("doge", -1.5, t0 + CHOICE_SAMPLE_MS);
		expect(openChoiceTrail("doge")).toHaveLength(2);
		expect(openChoiceTrail("doge")[1].t).toBe(t0 + CHOICE_SAMPLE_MS);
	});
});

describe("choice window", () => {
	it("slides by the elapsed time and keeps the same span", () => {
		const first = choiceWindow(1_000_000);
		const later = choiceWindow(1_000_000 + 30_000);

		expect(first.span).toBe(CHOICE_WINDOW_MS);
		expect(later.span).toBe(first.span);
		expect(later.from - first.from).toBe(30_000);
		expect(later.through - first.through).toBe(30_000);
	});

	it("holds the last return on the left edge", () => {
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
			{ t: 100, pnl: 2 },
			{ t: 150, pnl: 3 },
		]);
	});

	it("steps at the sample instead of ramping across the gap", () => {
		expect(
			stepPath(
				[
					{ t: 0, pnl: 1 },
					{ t: 10, pnl: 4 },
				],
				(t) => t,
				(pnl) => pnl,
			),
		).toBe("M0.0,1.0 L10.0,1.0 L10.0,4.0");
	});
});
