import { describe, expect, it } from "vitest";

import type { StatusEvent } from "#/lib/symm/events";
import {
	positionSymbolsFromStatus,
	sortOpenPositions,
} from "#/lib/symm/positions";
import { pruneClosedTradeEnters } from "#/lib/symm/stores/status-store";
import { selectTradePanelRows } from "#/lib/symm/trade-panel";

describe("pruneClosedTradeEnters", () => {
	it("removes closed trade_enter rows", () => {
		const trades = pruneClosedTradeEnters(
			[
				{
					event: "trade_enter",
					ts: "2026-05-23T12:00:00.000000000Z",
					symbol: "DEEP/EUR",
					regime: "flow",
					reason: "ok",
					score: 1,
					trail_pct: 0.01,
					fill: 1,
					stop: 0.9,
					notional_eur: 10,
					last: 1,
				},
				{
					event: "trade_exit",
					ts: "2026-05-23T12:01:00.000000000Z",
					symbol: "Q/EUR",
					regime: "pump",
					reason: "stop_ratchet",
					pnl_eur: -0.1,
					hold_ms: 60_000,
					entry_price: 1,
					stop_price: 0.9,
					peak_price: 1,
				},
			],
			new Set(["ZETA/EUR"]),
		);

		expect(trades.map((trade) => trade.event)).toEqual(["trade_exit"]);
	});
});

describe("sortOpenPositions", () => {
	it("orders open positions by opened_at ascending", () => {
		const ordered = sortOpenPositions([
			{
				symbol: "Z/EUR",
				regime: "pump",
				entry_price: 1,
				stop_price: 0.9,
				peak_price: 1,
				trail_pct: 0.01,
				notional_eur: 10,
				opened_at: "2026-05-23T12:02:00.000000000Z",
			},
			{
				symbol: "A/EUR",
				regime: "pump",
				entry_price: 1,
				stop_price: 0.9,
				peak_price: 1,
				trail_pct: 0.01,
				notional_eur: 10,
				opened_at: "2026-05-23T12:00:00.000000000Z",
			},
		]);

		expect(ordered.map((position) => position.symbol)).toEqual([
			"A/EUR",
			"Z/EUR",
		]);
	});

	it("positionSymbolsFromStatus preserves entry order", () => {
		expect(
			positionSymbolsFromStatus({
				event: "status",
				ts: "2026-05-23T12:03:00.000000000Z",
				equity_eur: 200,
				cash_eur: 190,
				closed_pnl_eur: 0,
				trade_count: 0,
				win_rate: 0,
				open_count: 2,
				positions: [
					{
						symbol: "Z/EUR",
						regime: "pump",
						entry_price: 1,
						stop_price: 0.9,
						peak_price: 1,
						trail_pct: 0.01,
						notional_eur: 10,
						opened_at: "2026-05-23T12:02:00.000000000Z",
					},
					{
						symbol: "A/EUR",
						regime: "pump",
						entry_price: 1,
						stop_price: 0.9,
						peak_price: 1,
						trail_pct: 0.01,
						notional_eur: 10,
						opened_at: "2026-05-23T12:00:00.000000000Z",
					},
				],
			} satisfies StatusEvent),
		).toEqual(["A/EUR", "Z/EUR"]);
	});
});

describe("selectTradePanelRows", () => {
	it("shows only open positions and exit history", () => {
		const rows = selectTradePanelRows({
			trades: [
				{
					event: "trade_enter",
					ts: "2026-05-23T15:30:00.000000000Z",
					symbol: "DEEP/EUR",
					regime: "pump",
					reason: "precursor",
					score: 0.9,
					trail_pct: 0.01,
					fill: 1,
					stop: 0.9,
					notional_eur: 10,
					last: 1,
				},
				{
					event: "trade_exit",
					ts: "2026-05-23T15:31:00.000000000Z",
					symbol: "Q/EUR",
					regime: "pump",
					reason: "stop_ratchet",
					pnl_eur: -0.1,
					hold_ms: 60_000,
					entry_price: 1,
					stop_price: 0.9,
					peak_price: 1,
				},
			],
			status: {
				event: "status",
				ts: "2026-05-23T15:32:00.000000000Z",
				equity_eur: 200,
				cash_eur: 190,
				closed_pnl_eur: 0,
				trade_count: 1,
				win_rate: 0,
				open_count: 1,
				positions: [
					{
						symbol: "ZETA/EUR",
						regime: "pump",
						entry_price: 0.04,
						stop_price: 0.03,
						peak_price: 0.04,
						last_price: 0.04,
						trail_pct: 0.01,
						notional_eur: 10,
						opened_at: "2026-05-23T15:30:00.000000000Z",
					},
				],
			} satisfies StatusEvent,
		});

		expect(rows.map((row) => `${row.kind}:${row.symbol}`)).toEqual([
			"open:ZETA/EUR",
			"exit:Q/EUR",
		]);
	});
});
