import { describe, expect, it } from "vitest";
import { CHOICE_TRAIL_LENGTH } from "./open-choice-draw";

describe("open-choice-draw", () => {
	it("exports the fifo window length SciChart uses", () => {
		expect(CHOICE_TRAIL_LENGTH).toBe(90);
	});
});
