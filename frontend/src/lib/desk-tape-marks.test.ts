import { describe, expect, it } from "vitest";
import {
	lifecycleFromDeskClosed,
	lifecycleFromDeskOpen,
	shadowComparisonPending,
} from "./desk-closed-lifecycle";
import { policyEpochFromRow, tapeMarksFromDeskRow } from "./desk-tape-marks";

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
			learning: { exit_at: "2026-10-23T20:05:40Z" },
			pnl: -0.046,
			pnl_pct: -4.6,
			outcome_triggers: ["cleared friction"],
		});

		expect(life?.marks_a).toBe(36300);
		expect(life?.marks_b).toBe(36340);
		expect(life?.marks_c).toBe(36349);
		expect(life?.policy_epoch).toBe("1792169333015760400");
		expect(life?.events?.[0]?.detail).toContain("tick 36340");
		expect(life?.events?.[1]?.detail).toContain("cleared friction");
	});

	it("waits for the learning shadow before promoting a closed row", () => {
		const pending = {
			id: "lot-2",
			symbol: "ETH/USD",
			virtual: true,
			opened_at: "2026-10-23T20:04:08Z",
			closed_at: "2026-10-23T20:05:28Z",
			official_exit_at: "2026-10-23T20:05:28Z",
			official_exit_pct: 0.4,
			learning: { active: true },
			pnl_pct: 0.4,
		};

		expect(shadowComparisonPending(pending)).toBe(true);
		expect(lifecycleFromDeskClosed(pending)).toBeNull();
		expect(lifecycleFromDeskOpen(pending)?.status).toBe("holding");

		const done = {
			...pending,
			learning: {
				active: false,
				exit_at: "2026-10-23T20:06:00Z",
				exit_pct: 0.9,
			},
		};
		expect(shadowComparisonPending(done)).toBe(false);
		expect(lifecycleFromDeskClosed(done)?.status).toBe("closed");
	});

	it("does not block a settled lot that has no live learning shadow", () => {
		const orphan = {
			id: "lot-3",
			symbol: "NIGHT/USD",
			virtual: true,
			opened_at: "2026-10-23T20:04:08Z",
			closed_at: "2026-10-23T20:05:28Z",
			official_exit_at: "2026-10-23T20:05:28Z",
			official_exit_pct: 0.4,
			learning: { active: false },
			pnl_pct: 0.4,
		};

		expect(shadowComparisonPending(orphan)).toBe(false);
		expect(lifecycleFromDeskClosed(orphan)?.status).toBe("closed");
	});
});

describe("policyEpochFromRow", () => {
	it("reads policy_epoch as a string for commit", () => {
		expect(policyEpochFromRow({ policy_epoch: 42 })).toBe("42");
	});
});
