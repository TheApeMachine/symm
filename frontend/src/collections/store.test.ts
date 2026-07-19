import { describe, expect, it } from "vitest";
import { latestOf } from "./circular";
import { createKeyedStore } from "./store";

type Row = { symbol: string; source: string; value: number };

const row = (symbol: string, source: string, value: number): Row => ({
	symbol,
	source,
	value,
});

describe("createKeyedStore", () => {
	it("keeps a single buffer under the store name when no keys are given", () => {
		const store = createKeyedStore<Row>()("tick", 2);

		store.actions.updateFrame([
			row("BTC/USD", "w", 1),
			row("ETH/USD", "w", 2),
			row("SOL/USD", "w", 3),
		]);

		expect(store.state.tick.values().map((entry) => entry.value)).toEqual([
			2, 3,
		]);
		expect(latestOf(store.state.tick)?.value).toBe(3);
	});

	it("nests a single key into one buffer per key value", () => {
		const store = createKeyedStore<Row>()("holdings", 2, (r) => r.symbol);

		store.actions.updateFrame([
			row("BTC/USD", "wallet", 1),
			row("BTC/USD", "wallet", 2),
			row("ETH/USD", "wallet", 3),
		]);

		expect(store.state.holdings["BTC/USD"].values()).toHaveLength(2);
		expect(latestOf(store.state.holdings["BTC/USD"])?.value).toBe(2);
		expect(latestOf(store.state.holdings["ETH/USD"])?.value).toBe(3);
		expect(store.state.version).toBe(1);
	});

	it("adds 1 to version on each write so subscribers are notified", () => {
		const store = createKeyedStore<Row>()("tick", 4);
		const versions: number[] = [];

		const subscription = store.subscribe((state) => {
			versions.push(state.version);
		});

		store.actions.updateFrame([row("BTC/USD", "w", 1)]);
		store.actions.updateFrame([row("BTC/USD", "w", 2)]);

		expect(store.state.version).toBe(2);
		expect(versions).toEqual([1, 2]);

		subscription.unsubscribe();
	});

	it("nests two keys into a Record of Records of buffers", () => {
		const store = createKeyedStore<Row>()(
			"measurements",
			50,
			(r) => r.symbol,
			(r) => r.source,
		);

		store.actions.updateFrame([
			row("BTC/USD", "leadlag", 0.1),
			row("BTC/USD", "hawkes", 0.2),
			row("ETH/USD", "leadlag", 0.3),
		]);

		expect(latestOf(store.state.measurements["BTC/USD"]?.leadlag)?.value).toBe(
			0.1,
		);
		expect(latestOf(store.state.measurements["BTC/USD"]?.hawkes)?.value).toBe(
			0.2,
		);
		expect(latestOf(store.state.measurements["ETH/USD"]?.leadlag)?.value).toBe(
			0.3,
		);
	});

	it("bounds each leaf buffer to the configured limit", () => {
		const store = createKeyedStore<Row>()("holdings", 2, (r) => r.symbol);

		store.actions.updateFrame([
			row("BTC/USD", "w", 1),
			row("BTC/USD", "w", 2),
			row("BTC/USD", "w", 3),
		]);

		expect(
			store.state.holdings["BTC/USD"].values().map((r) => r.value),
		).toEqual([2, 3]);
	});

	it("treats an empty batch as a no-op that preserves state identity", () => {
		const store = createKeyedStore<Row>()("holdings", 2, (r) => r.symbol);

		store.actions.updateFrame([row("BTC/USD", "w", 1)]);
		const before = store.state;
		store.actions.updateFrame([]);

		expect(store.state).toBe(before);
		expect(store.state.version).toBe(1);
	});

	it("preserves the nested container reference across updates", () => {
		const store = createKeyedStore<Row>()("holdings", 2, (r) => r.symbol);

		store.actions.updateFrame([row("BTC/USD", "w", 1)]);
		const mapBefore = store.state.holdings;
		store.actions.updateFrame([row("BTC/USD", "w", 2)]);

		expect(store.state.holdings).toBe(mapBefore);
	});

	it("clears buffers and version on reset", () => {
		const store = createKeyedStore<Row>()("holdings", 2, (r) => r.symbol);

		store.actions.updateFrame([row("BTC/USD", "w", 1)]);
		store.actions.reset();

		expect(store.state.holdings).toEqual({});
		expect(store.state.version).toBe(0);
	});
});
