import { describe, expect, it } from "vitest";
import { walletMetrics } from "./panels";

describe("walletMetrics", () => {
	it("adds funded cost basis and fee-inclusive PnL to remaining quote cash", () => {
		expect(
			walletMetrics(
				[{ asset: "USD", balance: 598.44, available: 598.44, reserved: 0 }],
				[
					{
						symbol: "BTC/USD",
						qty: 0.01,
						entry_price: 60000,
						entry_fee: 1.56,
						exit_fee: 1.586,
						mark: 61000,
						pnl: 10,
						return_pct: 1 / 60,
					},
				],
			),
		).toEqual({
			asset: "USD",
			cash: 598.44,
			available: 598.44,
			reserved: 0,
			unrealized: 10,
			equity: 1210,
		});
	});

	it("rejects ambiguous quote balance snapshots", () => {
		expect(() =>
			walletMetrics(
				[
					{ asset: "USD", balance: 1200, available: 1100, reserved: 100 },
					{ asset: "EUR", balance: 20, available: 20, reserved: 0 },
				],
				[],
			),
		).toThrow("exactly one quote balance");
	});
});
