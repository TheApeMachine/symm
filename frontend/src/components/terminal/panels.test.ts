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

	it("keeps equity at initial plus fee-inclusive PnL when cash paid the entry fee", () => {
		// Live gap: cash already debited cost+fee, PnL includes entry fee, so
		// committed must include entry_fee or equity undershoots initial+PnL.
		const initial = 200;
		const qty = 488.8;
		const entry = 0.0463;
		const entryFee = 0.2209;
		const pnl = 0.0509;
		const cash = initial - entry * qty - entryFee;
		const metrics = walletMetrics(
			[{ asset: "USD", balance: cash, available: cash, reserved: 0 }],
			[
				{
					symbol: "ESPORTS/USD",
					qty,
					entry_price: entry,
					entry_fee: entryFee,
					exit_fee: 0,
					mark: 0.0469,
					pnl,
					return_pct: pnl / (entry * qty + entryFee),
				},
			],
		);

		expect(metrics?.equity).toBeCloseTo(initial + pnl, 4);
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
