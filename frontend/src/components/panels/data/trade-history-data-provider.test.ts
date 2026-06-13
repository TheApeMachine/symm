import { beforeEach, describe, expect, it } from "vitest";

import { TradeHistoryDataProvider } from "#/components/panels/data/trade-history-data-provider";

describe("TradeHistoryDataProvider", () => {
	beforeEach(() => {
		TradeHistoryDataProvider.reset();
	});

	it("records a closed trade when inventory drops to zero", () => {
		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: { H: 26.29 },
			AvgEntry: { H: 0.24 },
			Unrealized: { H: 0.13145 },
		});

		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: {},
		});

		const row = TradeHistoryDataProvider.snapshot()[0];

		expect(row).toMatchObject({
			symbol: "H/EUR",
			qty: 26.29,
			entryPrice: 0.24,
			realizedEur: 0.13145,
			outcome: "profit",
			reason: "position closed",
		});
	});

	it("finalizes audit exit rows with actual return", () => {
		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: { BOBA: 20 },
			AvgEntry: { BOBA: 0.05 },
		});

		TradeHistoryDataProvider.ingestAudit({
			event: "audit",
			audit_event: "exit",
			seq: 3,
			ts: "2026-05-29T01:02:05Z",
			symbol: "BOBA/EUR",
			reason: "perspective TTL elapsed",
			actual_return: -0.0973,
			success: false,
			slot_eur: 1,
		});

		const row = TradeHistoryDataProvider.snapshot()[0];

		expect(row).toMatchObject({
			symbol: "BOBA/EUR",
			outcome: "loss",
			reason: "perspective TTL elapsed",
		});
		expect(row?.realizedEur).toBeCloseTo(-0.0973, 8);
		expect(row?.realizedPct).toBeCloseTo(-9.73, 8);
	});

	it("keeps newest closed trades first", () => {
		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: { A: 1 },
			AvgEntry: { A: 1 },
			Unrealized: { A: 0.1 },
		});
		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: {},
		});

		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: { B: 2 },
			AvgEntry: { B: 2 },
			Unrealized: { B: -0.2 },
		});
		TradeHistoryDataProvider.ingestBalance({
			type: "balances",
			Currency: "EUR",
			Inventory: {},
		});

		const rows = TradeHistoryDataProvider.snapshot();

		expect(rows).toHaveLength(2);
		expect(rows[0]?.symbol).toBe("B/EUR");
		expect(rows[1]?.symbol).toBe("A/EUR");
	});
});
