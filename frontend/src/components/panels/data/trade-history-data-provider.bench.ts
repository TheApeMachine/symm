import { bench, describe } from "vitest";

import { TradeHistoryDataProvider } from "#/components/panels/data/trade-history-data-provider";

describe("TradeHistoryDataProvider", () => {
	bench("tracks inventory open and close cycles", () => {
		TradeHistoryDataProvider.reset();

		for (let index = 0; index < 8; index += 1) {
			const base = `SYM${index}`;

			TradeHistoryDataProvider.ingestBalance({
				type: "balances",
				Currency: "EUR",
				Inventory: { [base]: 10 + index },
				AvgEntry: { [base]: 0.2 + index * 0.01 },
				Unrealized: { [base]: index % 2 === 0 ? 0.05 : -0.03 },
			});

			TradeHistoryDataProvider.ingestBalance({
				type: "balances",
				Currency: "EUR",
				Inventory: {},
			});
		}

		if (TradeHistoryDataProvider.snapshot().length !== 8) {
			throw new Error("expected eight closed trades");
		}
	});
});
