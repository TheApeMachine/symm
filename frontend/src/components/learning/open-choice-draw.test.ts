import { describe, expect, it } from "vitest";
import type { Position } from "#/collections/types";
import { CHOICE_TRAIL_LENGTH, chartHeld } from "./open-choice-draw";

describe("open-choice chartHeld", () => {
	it("keeps open paper lots and drops closed lots", () => {
		const open = {
			id: "a",
			symbol: "DOGE/USD",
			entry_path: "",
			exit_path: "",
			opened_at: "2026-01-01T00:00:00Z",
		} as Position;

		expect(chartHeld(open, "paper")).toBe(true);
		expect(chartHeld(open, "virtual")).toBe(false);

		const closed = {
			...open,
			id: "b",
			closed_at: "2026-01-01T00:01:00Z",
			pnl_pct: -1,
		} as Position;

		expect(chartHeld(closed, "paper")).toBe(false);
	});

	it("exports the fifo window length SciChart uses", () => {
		expect(CHOICE_TRAIL_LENGTH).toBe(90);
	});
});
