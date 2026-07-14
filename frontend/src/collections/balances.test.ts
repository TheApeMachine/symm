import { describe, expect, it } from "vitest";
import { balancesStore, normalizeBalances } from "./balances";

describe("normalizeBalances", () => {
	it("normalizes the complete quote balance snapshot", () => {
		expect(
			normalizeBalances([
				{
					asset: "USD",
					balance: "1200.50",
					available: "1100.25",
					reserved: "100.25",
					ledger_id: "ledger-1",
				},
			]),
		).toEqual([
			{
				asset: "USD",
				balance: 1200.5,
				available: 1100.25,
				reserved: 100.25,
				ledger_id: "ledger-1",
			},
		]);
	});

	it("rejects a snapshot missing required financial fields", () => {
		expect(() =>
			normalizeBalances([{ asset: "USD", balance: 1200, available: 1100 }]),
		).toThrow("balances[0].reserved");
	});

	it("does not coerce null financial fields to zero", () => {
		expect(() =>
			normalizeBalances([
				{ asset: "USD", balance: null, available: 1100, reserved: 100 },
			]),
		).toThrow("balances[0].balance");
	});

	it("retains the last quote snapshot when a bundled desk frame omits balances", () => {
		balancesStore.actions.updateFrame([
			{
				asset: "USD",
				balance: 1200,
				available: 1100,
				reserved: 100,
			},
		]);
		balancesStore.actions.updateFrame([]);

		expect(balancesStore.state.balances).toEqual([
			{
				asset: "USD",
				balance: 1200,
				available: 1100,
				reserved: 100,
			},
		]);
	});
});
