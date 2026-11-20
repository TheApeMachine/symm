import { describe, expect, it } from "vitest";
import { STOPLOSS_ARMED, STOPLOSS_HEALTHY } from "#/collections/types";
import { chartHeld, CHOICE_TRAIL_LENGTH } from "./open-choice-draw";

describe("open-choice-draw", () => {
	it("exports the fifo window length SciChart uses", () => {
		expect(CHOICE_TRAIL_LENGTH).toBe(90);
	});

	it("chartHeld keeps open rows on the selected book", () => {
		expect(
			chartHeld(
				{
					id: "a",
					symbol: "XBT/USD",
					entry_path: "",
					exit_path: "",
					opened_at: "2026-01-01T00:00:00Z",
					tier_key: "real",
					pnl_pct: 1.2,
					stoploss: {
						State: STOPLOSS_ARMED,
						InitialPnLPct: 0,
						CurrentPnLPct: 1.2,
						PeakPnLPct: 2,
						ArmPnlPct: 1,
						ExhaustScore: 0,
						Confirms: 0,
					},
				},
				"real",
			),
		).toBe(true);

		expect(
			chartHeld(
				{
					id: "b",
					symbol: "ETH/USD",
					entry_path: "",
					exit_path: "",
					opened_at: "2026-01-01T00:00:00Z",
					closed_at: "2026-01-01T01:00:00Z",
					tier_key: "real",
					stoploss: {
						State: STOPLOSS_HEALTHY,
						InitialPnLPct: 0,
						CurrentPnLPct: 0.2,
						PeakPnLPct: 0.2,
						ArmPnlPct: 1,
						ExhaustScore: 0,
						Confirms: 0,
					},
				},
				"real",
			),
		).toBe(false);
	});
});
