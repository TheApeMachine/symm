import { beforeEach, describe, expect, it } from "vitest";

import { TradesDataProvider } from "#/components/panels/data/trades-data-provider";

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

	it("returns a stable snapshot reference until data changes", () => {
		TradesDataProvider.ingest({
			Type: "paper",
			Currency: "EUR",
			Balance: 193.69,
			Inventory: { H: 26.29 },
			AvgEntry: { H: 0.24 },
			Marks: { "H/EUR": 0.24 },
		});

		const first = TradesDataProvider.snapshot();
		const second = TradesDataProvider.snapshot();

		expect(second).toBe(first);

		TradesDataProvider.setMark("H/EUR", 0.245);

		expect(TradesDataProvider.snapshot()).not.toBe(first);
	});

	it("ingests positions frames and populates open trade rows with authoritative P&L metrics", () => {
		TradesDataProvider.ingest({
			type: "positions",
			currency: "USD",
			cash: 149.87,
			open_positions: 1,
			priced_positions: 1,
			exit_value: 50.63,
			exit_balance: 0.5,
			liquidation_balance: 200.5,
			liquidation_complete: true,
			in_profit: true,
			positions: [
				{
					symbol: "BTC/USD",
					qty: 0.001,
					avg_entry: 50130,
					mark: 50630,
					exit_value: 50.63,
					unrealized: 0.5,
					unrealized_pct: 0.9974077,
					priced: true,
					stop_price: 49870,
					peak_price: 50700,
					offset: 0.015,
					mark_source: "stop_monitor",
				},
			],
		});

		const rows = TradesDataProvider.snapshot();
		expect(rows).toHaveLength(1);
		expect(rows[0]?.kind).toBe("open");
		expect(rows[0]?.symbol).toBe("BTC/USD");
		expect(rows[0]?.qty).toBe(0.001);
		expect(rows[0]?.entryPrice).toBe(50130);
		expect(rows[0]?.markPrice).toBe(50630);
		expect(rows[0]?.unrealizedEur).toBe(0.5);
		expect(rows[0]?.unrealizedPct).toBe(0.9974077);
	});
});
