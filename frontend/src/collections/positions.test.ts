import { describe, expect, it } from "vitest";
import { normalizePositions, positionsStore } from "./positions";

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

	it("defaults missing mark and entry fields so restart rows still render", () => {
		expect(
			normalizePositions([
				{
					symbol: "XLM/USD",
					qty: 10,
					entry_price: null,
					mark: 0.27,
				},
			]),
		).toEqual([
			{
				symbol: "XLM/USD",
				qty: 10,
				entry_price: 0.27,
				entry_fee: 0,
				exit_fee: 0,
				mark: 0.27,
				pnl: 0,
				return_pct: 0,
			},
		]);
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

describe("positionsStore", () => {
	it("clears inventory on an authoritative empty array snapshot", () => {
		positionsStore.actions.updateFrame([
			{
				symbol: "XLM/USD",
				qty: 10,
				entry_price: 0.25,
				entry_fee: 0.0065,
				exit_fee: 0.00702,
				mark: 0.27,
				pnl: 0.2,
				return_pct: 0.08,
			},
		]);

		positionsStore.actions.updateFrame([]);

		expect(positionsStore.state.positions).toHaveLength(0);
		expect(positionsStore.state.observed).toBe(true);
	});

	it("preserves inventory when a non-array incomplete frame arrives", () => {
		positionsStore.actions.updateFrame([
			{
				symbol: "XLM/USD",
				qty: 10,
				entry_price: 0.25,
				entry_fee: 0.0065,
				exit_fee: 0.00702,
				mark: 0.27,
				pnl: 0.2,
				return_pct: 0.08,
			},
		]);

		expect(() =>
			positionsStore.actions.updateFrame({ partial: true }),
		).not.toThrow();
		expect(positionsStore.state.positions).toHaveLength(1);
		expect(positionsStore.state.positions[0]?.symbol).toBe("XLM/USD");
	});
});
