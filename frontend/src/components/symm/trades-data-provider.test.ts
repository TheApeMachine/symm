import { describe, expect, it } from "vitest";

import { TradesDataProvider } from "#/components/symm/trades-data-provider";

describe("TradesDataProvider", () => {
	it("prefers live ticker marks over stale wallet marks", () => {
		TradesDataProvider.setMark("MNT/EUR", 0.57);

		TradesDataProvider.ingest({
			Type: "paper",
			Currency: "EUR",
			Balance: 198.9,
			Inventory: { MNT: 10 },
			AvgEntry: { MNT: 0.55 },
			Marks: { "MNT/EUR": 0.55 },
		});

		const open = TradesDataProvider.snapshot().find(
			(row) => row.kind === "open",
		);

		expect(open?.markPrice).toBe(0.57);
		expect(open?.unrealizedPct).toBeCloseTo(((0.57 - 0.55) / 0.55) * 100, 5);
	});
});
