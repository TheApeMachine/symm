import { bench, describe } from "vitest";

import { TradesDataProvider } from "#/components/symm/trades-data-provider";

const CURRENCY = "EUR";
const PAPER_BALANCE_EUR = 193.69;

const TRADE_FIXTURES = [
	{
		base: "H",
		symbol: "H/EUR",
		qty: 26.29,
		entryPrice: 0.24,
		markPrice: 0.245,
	},
	{
		base: "GFI",
		symbol: "GFI/EUR",
		qty: 62.5,
		entryPrice: 0.1129,
		markPrice: 0.1131,
	},
	{
		base: "PEAQ",
		symbol: "PEAQ/EUR",
		qty: 20,
		entryPrice: 0.0286,
		markPrice: 0.0285,
	},
	{
		base: "XLM",
		symbol: "XLM/EUR",
		qty: 45,
		entryPrice: 0.1451,
		markPrice: 0.1457,
	},
] as const;

const ingestFills = () => {
	for (const trade of TRADE_FIXTURES) {
		TradesDataProvider.ingest({
			OrderID: `entry-${trade.base}`,
			Symbol: trade.symbol,
			Side: "buy",
			Qty: trade.qty,
			Price: trade.entryPrice,
		});
	}
};

const ingestInventory = () => {
	TradesDataProvider.ingest({
		Type: "paper",
		Currency: CURRENCY,
		Balance: PAPER_BALANCE_EUR,
		Inventory: Object.fromEntries(
			TRADE_FIXTURES.map((trade) => [trade.base, trade.qty]),
		),
		AvgEntry: Object.fromEntries(
			TRADE_FIXTURES.map((trade) => [trade.base, trade.entryPrice]),
		),
		Marks: Object.fromEntries(
			TRADE_FIXTURES.map((trade) => [trade.symbol, trade.entryPrice]),
		),
	});
};

const refreshMarks = () => {
	for (const trade of TRADE_FIXTURES) {
		TradesDataProvider.setMark(trade.symbol, trade.markPrice);
	}
};

describe("TradesDataProvider", () => {
	bench("syncs open rows and ignores fills", () => {
		TradesDataProvider.reset();
		ingestFills();
		ingestInventory();
		refreshMarks();

		const rows = TradesDataProvider.snapshot();

		if (rows.length !== TRADE_FIXTURES.length) {
			throw new Error(`expected ${TRADE_FIXTURES.length} visible rows`);
		}
	});
});
