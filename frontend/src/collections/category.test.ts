import { describe, expect, it } from "vitest";
import { RingBuffer, signals } from "./app";
import { Value } from "#/types/value";

describe("categoryStore", () => {
	const categoryStore = signals.category;

	it("initializes as an empty record of symbol ring buffers", () => {
		expect(categoryStore).toBeDefined();
		expect(typeof categoryStore.state).toBe("object");
	});

	it("stores Value objects per symbol ring buffer", () => {
		const symbol = "BTC/USD";
		if (!categoryStore.state[symbol]) {
			categoryStore.state[symbol] = new RingBuffer<Value>(50);
		}

		const val = new Value({
			origin: "category",
			scope: symbol,
			tick: 100n,
			timestamp: 1718000000000000000n,
			metrics: [
				{
					name: "loaded_imbalance",
					raw: 0.85,
					snm: 0.85,
				},
			],
		});

		categoryStore.state[symbol].add(val);
		categoryStore.setState((prev) => ({ ...prev }));

		const ring = categoryStore.state[symbol];
		expect(ring.isEmpty()).toBe(false);
		expect(ring.getLast()).toEqual(val);
		expect(ring.getLast()?.origin).toBe("category");
	});

	it("integrates seamlessly with signals.category", () => {
		const symbol = "ETH/USD";
		const store = signals.category;
		expect(store).toBeDefined();
		if (!store.state[symbol]) {
			store.state[symbol] = new RingBuffer<Value>(50);
		}

		const val = new Value({
			origin: "category",
			scope: symbol,
			tick: 101n,
			timestamp: 1718000000000000000n,
		});

		store.state[symbol].add(val);
		expect(store.state[symbol].isEmpty()).toBe(false);
		expect(store.state[symbol].getLast()?.scope).toBe("ETH/USD");
		expect(categoryStore.state[symbol]?.getLast()?.tick).toBe(101n);
	});
});
