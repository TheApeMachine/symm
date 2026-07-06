import { bench, describe } from "vitest";
import type { Order } from "./orders";
import { ordersStore } from "./orders";

const orders: Order[] = Array.from({ length: 32 }, (_, index) => ({
	id: `PAPER-${String(index).padStart(5, "0")}`,
	pair: "BTCUSD",
	price: 90000 + index,
	reserved_amount: 9 + index / 100,
	reserved_asset: "USD",
	side: "buy",
	type: "limit",
	volume: 0.0001,
	created_at: "2026-07-06T10:00:00Z",
}));

describe("ordersStore", () => {
	bench("replaces the current order snapshot", () => {
		ordersStore.actions.updateFrame(orders);
	});
});
