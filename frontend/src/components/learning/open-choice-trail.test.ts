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
	it("appends one sample per store update even when return is flat", () => {
		resetOpenChoices();
		const t0 = 1_000_000;
		noteOpenChoice("doge", -1.5, t0);
		noteOpenChoice("doge", -1.5, t0 + 1);
		expect(openChoiceTrail("doge")).toHaveLength(2);
		expect(openChoiceTrail("doge")[1].t).toBe(t0 + 1);
	});

	it("exposes the same values array the card Trail uses", () => {
		resetOpenChoices();
		noteOpenChoice("doge", 1.2, 1_000);
		noteOpenChoice("doge", -0.5, 1_001);
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
					{ t: 1, pnl: 1 },
					{ t: 2, pnl: 3 },
				],
				[{ t: 2, pnl: 5 }],
			]),
		).toEqual([1, 4]);
	});

	it("caps the ring buffer at CHOICE_TRAIL_LENGTH", () => {
		resetOpenChoices();
		for (let i = 0; i < CHOICE_TRAIL_LENGTH + 5; i++) {
			noteOpenChoice("doge", i, 1_000 + i);
		}
		expect(openChoiceTrail("doge")).toHaveLength(CHOICE_TRAIL_LENGTH);
		expect(openChoiceTrail("doge")[0].pnl).toBe(5);
	});
});
