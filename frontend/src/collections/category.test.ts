import { describe, expect, it } from "vitest";
import { signals } from "./app";
import { Value } from "#/types/value";

describe("signals.category", () => {
	const categoryStore = signals.category;

	it("initializes as an empty record", () => {
		expect(categoryStore).toBeDefined();
		expect(typeof categoryStore.state).toBe("object");
	});

	it("stores one Value per symbol scope", () => {
		const symbol = "BTC-USD";

		const val = new Value({
			origin: "category",
			scope: symbol,
			tick: 100n,
			timestamp: 1718000000000000000n,
		});

		categoryStore.setState((prev) => ({ ...prev, [symbol]: val }));

		expect(categoryStore.state[symbol]).toEqual(val);
		expect(categoryStore.state[symbol]?.origin).toBe("category");
	});

	it("integrates seamlessly with signals.category", () => {
		const symbol = "ETH-USD";
		const store = signals.category;
		expect(store).toBeDefined();

		const val = new Value({
			origin: "category",
			scope: symbol,
			tick: 101n,
			timestamp: 1718000000000000000n,
		});

		store.setState((prev) => ({ ...prev, [symbol]: val }));

		expect(store.state[symbol]?.scope).toBe("ETH-USD");
		expect(categoryStore.state[symbol]?.tick).toBe(101n);
	});
});
