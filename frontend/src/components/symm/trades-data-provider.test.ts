import { beforeEach, describe, expect, it } from "vitest";

import { TradesDataProvider } from "#/components/symm/trades-data-provider";

describe("TradesDataProvider", () => {
	beforeEach(() => {
		TradesDataProvider.reset();
	});

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

	it("shows only open cards and never renders entry fills", () => {
		TradesDataProvider.ingest({
			OrderID: "entry-1",
			Symbol: "H/EUR",
			Side: "buy",
			Qty: 26.29,
			Price: 0.24,
		});

		TradesDataProvider.ingest({
			Type: "paper",
			Currency: "EUR",
			Balance: 193.69,
			Inventory: { H: 26.29 },
			AvgEntry: { H: 0.24 },
			Marks: { "H/EUR": 0.24 },
		});

		const rows = TradesDataProvider.snapshot();

		expect(rows).toHaveLength(1);
		expect(rows[0]?.kind).toBe("open");
		expect(rows[0]?.symbol).toBe("H/EUR");
	});

	it("does not show standalone entry fills", () => {
		TradesDataProvider.ingest({
			OrderID: "entry-1",
			Symbol: "MASK/EUR",
			Side: "buy",
			Qty: 10,
			Price: 0.42,
		});

		expect(TradesDataProvider.snapshot()).toEqual([]);
	});

	it("refreshes open-card profit and loss from live marks", () => {
		TradesDataProvider.ingest({
			Type: "paper",
			Currency: "EUR",
			Balance: 193.69,
			Inventory: { H: 26.29 },
			AvgEntry: { H: 0.24 },
			Marks: { "H/EUR": 0.24 },
		});

		TradesDataProvider.setMark("H/EUR", 0.245);

		const open = TradesDataProvider.snapshot()[0];

		expect(open?.kind).toBe("open");
		expect(open?.markPrice).toBe(0.245);
		expect(open?.unrealizedEur).toBeCloseTo(26.29 * 0.005, 8);
		expect(open?.unrealizedPct).toBeCloseTo((0.005 / 0.24) * 100, 8);
	});
});
