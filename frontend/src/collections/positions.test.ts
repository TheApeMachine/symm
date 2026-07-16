import { describe, expect, it } from "vitest";
import { normalizePositions } from "./positions";

describe("normalizePositions", () => {
	it("preserves complete snapshots while normalizing financial fields", () => {
		expect(
			normalizePositions([
				{
					symbol: "XLM/USD",
					qty: "10",
					entry_price: "0.25",
					entry_fee: "0.0065",
					exit_fee: "0.00702",
					mark: "0.27",
					pnl: "0.20",
					return_pct: "0.08",
					spread: "0.001",
					price_increment: "0.0001",
				},
			]),
		).toEqual([
			{
				symbol: "XLM/USD",
				qty: 10,
				entry_price: 0.25,
				entry_fee: 0.0065,
				exit_fee: 0.00702,
				mark: 0.27,
				pnl: 0.2,
				return_pct: 0.08,
				spread: "0.001",
				price_increment: "0.0001",
			},
		]);
	});

	it("rejects a snapshot missing the immediate mark", () => {
		expect(() =>
			normalizePositions([
				{
					symbol: "XLM/USD",
					qty: 10,
					entry_price: 0.25,
					entry_fee: 0.0065,
					exit_fee: 0.00702,
					pnl: 0.2,
					return_pct: 0.08,
				},
			]),
		).toThrow("positions[0].mark");
	});

	it("replaces invalid executions with an empty array", () => {
		expect(
			normalizePositions([
				{
					symbol: "XLM/USD",
					qty: 10,
					entry_price: 0.25,
					entry_fee: 0.0065,
					exit_fee: 0.00702,
					mark: 0.27,
					pnl: 0.2,
					return_pct: 0.08,
					executions: "invalid",
				},
			])[0]?.executions,
		).toEqual([]);
	});
});
