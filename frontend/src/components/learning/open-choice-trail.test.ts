import { describe, expect, it } from "vitest";
import {
	CHOICE_TRAIL_LENGTH,
	indexMean,
	linePath,
	noteOpenChoice,
	openChoiceTrail,
	openChoiceValues,
	resetOpenChoices,
} from "./open-choice-trail";

describe("open choice trail", () => {
	it("skips duplicate marks so flat ticks do not grow the buffer", () => {
		resetOpenChoices();
		noteOpenChoice("doge", -1.5, "mark-a");
		noteOpenChoice("doge", -1.5, "mark-a");
		expect(openChoiceTrail("doge")).toHaveLength(1);
		noteOpenChoice("doge", -1.5, "mark-b");
		expect(openChoiceTrail("doge")).toHaveLength(2);
	});

	it("exposes the same values array the card Trail uses", () => {
		resetOpenChoices();
		noteOpenChoice("doge", 1.2, "a");
		noteOpenChoice("doge", -0.5, "b");
		expect(openChoiceValues("doge")).toEqual([1.2, -0.5]);
	});

	it("joins samples in order the way the card trail does", () => {
		expect(
			linePath([
				{ x: 0, y: 1 },
				{ x: 10, y: 4 },
			]),
		).toBe("M0.0,1.0 L10.0,4.0");
	});

	it("right-aligns a shorter lot so now stays the last slot", () => {
		expect(
			indexMean([
				[
					{ t: 1, pnl: 1, stamp: "a" },
					{ t: 2, pnl: 3, stamp: "b" },
				],
				[{ t: 2, pnl: 5, stamp: "c" }],
			]),
		).toEqual([1, 4]);
	});

	it("caps the ring buffer at CHOICE_TRAIL_LENGTH", () => {
		resetOpenChoices();
		for (let i = 0; i < CHOICE_TRAIL_LENGTH + 5; i++) {
			noteOpenChoice("doge", i, `tick-${i}`);
		}
		expect(openChoiceTrail("doge")).toHaveLength(CHOICE_TRAIL_LENGTH);
		expect(openChoiceTrail("doge")[0].pnl).toBe(5);
	});
});
