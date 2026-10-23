import { describe, expect, it } from "vitest";
import { policyEpochFromRow, tapeMarksFromDeskRow } from "./desk-tape-marks";
import { lifecycleFromDeskClosed } from "./desk-closed-lifecycle";

describe("tapeMarksFromDeskRow", () => {
	it("maps entry and exit ticks to B and C with A from mark_a_tick", () => {
		expect(
			tapeMarksFromDeskRow({
				mark_a_tick: 100,
				entry_tick: 105,
				exit_tick: 120,
			}),
		).toEqual({ a: 100, b: 105, c: 120 });
	});

	it("uses entry as A when mark_a_tick is absent", () => {
		expect(
			tapeMarksFromDeskRow({
				entry_tick: 105,
				exit_tick: 120,
			}),
		).toEqual({ a: 105, b: 105, c: 120 });
	});
});

describe("lifecycleFromDeskClosed", () => {
	it("carries tape marks and policy epoch from the desk JSON", () => {
		const life = lifecycleFromDeskClosed({
			id: "lot-1",
			symbol: "DOS/USD",
			virtual: true,
			policy_epoch: 1792169333015760400,
			mark_a_tick: 36300,
			entry_tick: 36340,
			exit_tick: 36349,
			entry_path: "R01/R05",
			opened_at: "2026-10-23T20:04:08Z",
			closed_at: "2026-10-23T20:05:28Z",
			cost: 200,
			proceeds: 190.83,
			profit: -9.17,
			net_return: -4.6,
			outcome_triggers: ["cleared friction"],
		});

		expect(life?.marks_a).toBe(36300);
		expect(life?.marks_b).toBe(36340);
		expect(life?.marks_c).toBe(36349);
		expect(life?.policy_epoch).toBe("1792169333015760400");
		expect(life?.events?.[0]?.detail).toContain("tick 36340");
		expect(life?.events?.[1]?.detail).toContain("cleared friction");
	});
});

describe("policyEpochFromRow", () => {
	it("reads policy_epoch as a string for commit", () => {
		expect(policyEpochFromRow({ policy_epoch: 42 })).toBe("42");
	});
});
