import { describe, expect, it } from "vitest";
import { normalizeExecutions } from "./executions";

const trade = {
	exec_id: "E1",
	order_id: "O1",
	exec_type: "trade",
	order_status: "filled",
	symbol: "BTC/USD",
	side: "buy",
	last_qty: "0.01",
	last_price: "61420",
	timestamp: "2026-07-12T04:00:00Z",
};

describe("normalizeExecutions", () => {
	it("flattens Kraken envelopes and preserves envelope audit context", () => {
		expect(
			normalizeExecutions([
				{
					channel: "executions",
					type: "update",
					sequence: 42,
					data: [trade],
				},
			]),
		).toEqual([
			{
				...trade,
				last_qty: 0.01,
				last_price: 61420,
				channel: "executions",
				type: "update",
				sequence: 42,
			},
		]);
	});

	it("accepts already-flat semantic rows in the same snapshot", () => {
		expect(
			normalizeExecutions([trade, { ...trade, exec_id: "E2" }]),
		).toHaveLength(2);
	});

	it("rejects trade rows missing required execution financial fields", () => {
		expect(() =>
			normalizeExecutions([{ ...trade, last_price: undefined }]),
		).toThrow("executions[0].last_price");
	});
});
