import { describe, expect, it } from "vitest";
import {
	lifecycleFromDeskClosed,
	lifecycleFromDeskOpen,
} from "./desk-closed-lifecycle";
import { policyEpochFromRow, tapeMarksFromDeskRow } from "./desk-tape-marks";

describe("tapeMarksFromDeskRow", () => {
	it("maps entry and exit ticks with A equal to entry", () => {
		expect(
			tapeMarksFromDeskRow({
				entry_tick: 105,
				exit_tick: 120,
			}),
		).toEqual({ a: 105, b: 105, c: 120 });
	});

	it("rejects inverted exit ticks", () => {
		expect(
			tapeMarksFromDeskRow({
				entry_tick: 120,
				exit_tick: 105,
			}),
		).toBeNull();
	});
});

describe("lifecycleFromDeskClosed", () => {
	it("carries tape marks and policy epoch from the desk JSON", () => {
		const life = lifecycleFromDeskClosed({
			id: "lot-1",
			symbol: "DOS/USD",
			tier_key: "virtual",
			policy_epoch: 1792169333015760400,
			entry_tick: 36340,
			exit_tick: 36349,
			entry_path: "R01/R05",
			exit_path: "",
			opened_at: "2026-10-23T20:04:08Z",
			closed_at: "2026-10-23T20:05:28Z",
			pnl: -0.046,
			pnl_pct: -4.6,
			outcome_triggers: ["cleared friction"],
		});

		expect(life?.marks_a).toBe(36340);
		expect(life?.marks_b).toBe(36340);
		expect(life?.marks_c).toBe(36349);
		expect(life?.policy_epoch).toBe("1792169333015760400");
		expect(life?.events?.[0]?.detail).toContain("tick 36340");
		expect(life?.events?.[1]?.detail).toContain("cleared friction");
	});

	it("promotes a closed row immediately without a shadow wait", () => {
		const closed = {
			id: "lot-2",
			symbol: "ETH/USD",
			tier_key: "virtual",
			entry_path: "",
			exit_path: "",
			opened_at: "2026-10-23T20:04:08Z",
			closed_at: "2026-10-23T20:05:28Z",
			pnl_pct: 0.4,
		};

		expect(lifecycleFromDeskClosed(closed)?.status).toBe("closed");
		expect(lifecycleFromDeskOpen(closed)).toBeNull();
	});
});

describe("policyEpochFromRow", () => {
	it("reads policy_epoch as a string for commit", () => {
		expect(policyEpochFromRow({ policy_epoch: 42 })).toBe("42");
	});
});
