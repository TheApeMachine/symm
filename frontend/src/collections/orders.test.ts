import { describe, expect, it } from "vitest";
import type { Order } from "./orders";
import { ordersStore } from "./orders";

const order: Order = {
	id: "PAPER-00003",
	pair: "BTCUSD",
	price: 90000,
	reserved_amount: 9,
	reserved_asset: "USD",
	side: "buy",
	type: "limit",
	volume: 0.0001,
	created_at: "2026-07-06T10:00:00Z",
};

describe("ordersStore", () => {
	it("keeps the current backend order snapshot", () => {
		ordersStore.actions.updateFrame([order]);

		expect(ordersStore.state.orders).toEqual([order]);
		expect(ordersStore.state.observed).toBe(true);

		ordersStore.actions.updateFrame([]);

		expect(ordersStore.state.orders).toEqual([]);
	});
});
