import { describe, expect, it } from "vitest";
import {
	closedListingFromEnvelope,
	pathsListingFromEnvelope,
} from "./s3-prefix-compact";

describe("s3-prefix-compact", () => {
	it("reads explore thresholds from a paths envelope", () => {
		const compact = pathsListingFromEnvelope({
			scope: "model/paths",
			epoch: "7",
			thresholds: {
				"model/paths/R01/R02/enter.bin": 0.55,
				"model/paths/R03/R04/exit.bin": 0.9,
			},
		});

		expect(compact?.kind).toBe("explore");
		if (compact?.kind !== "explore") return;

		expect(compact.policyKeys).toEqual(["model/paths/R01/R02/enter.bin"]);
		expect(compact.thresholds["model/paths/R01/R02/enter.bin"]).toBe(0.55);
		expect(compact.policyKeys).not.toContain(
			"model/paths/R03/R04/exit.bin",
		);
		expect(compact.epoch).toBe("7");
	});

	it("reads closed lots from a closed envelope", () => {
		const compact = closedListingFromEnvelope({
			scope: "virtual/closed",
			entries: [
				{
					epoch: 1,
					tick: 5,
					timestamp: 9,
					row: {
						id: "hist-compact",
						symbol: "BTC/USD",
						opened_at: "2026-10-18T08:00:00Z",
						closed_at: "2026-10-18T09:00:00Z",
						qty: "1",
						entry_price: "1",
						mark: "1.05",
						pnl: 0.05,
						pnl_pct: 5,
					},
				},
			],
		});

		expect(compact?.kind).toBe("closed");
		if (compact?.kind !== "closed") return;

		expect(compact.entries).toHaveLength(1);
		expect(compact.entries[0]?.row.id).toBe("hist-compact");
		expect(compact.entries[0]?.row.tier_key).toBe("virtual");
		expect(compact.entries[0]?.stamp.tick).toBe(5n);
	});

	it("accepts paper/closed scopes from the desk ledger", () => {
		const compact = closedListingFromEnvelope({
			scope: "paper/closed",
			entries: [
				{
					epoch: 2,
					tick: 3,
					timestamp: 4,
					row: { id: "paper-lot", symbol: "ETH/USD" },
				},
			],
		});

		expect(compact?.kind).toBe("closed");
		expect(compact?.scope).toBe("paper/closed");
		expect(compact?.entries[0]?.row.id).toBe("paper-lot");
	});
});
