import { describe, expect, it } from "vitest";
import { categoryStore, getMeasurementStore, RingBuffer } from "./app";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

describe("categoryStore", () => {
	it("initializes as an empty record of symbol ring buffers", () => {
		expect(categoryStore).toBeDefined();
		expect(typeof categoryStore.state).toBe("object");
	});

	it("stores MeasurementT objects per symbol ring buffer", () => {
		const symbol = "BTC/USD";
		if (!categoryStore.state[symbol]) {
			categoryStore.state[symbol] = new RingBuffer<MeasurementT>(50);
		}

		const measurement: MeasurementT = {
			source: "category",
			symbol,
			tick: 100n,
			at: 1718000000000000000n,
			observedFrom: 1718000000000000000n,
			horizon: 0n,
			peerAt: 0n,
			peerObservedFrom: 0n,
			maturity: 1.0,
			snr: 0.95,
			snrDefined: true,
			metrics: [
				{
					name: "loaded_imbalance",
					raw: 0.85,
					normalized: 0.85,
					hasNormalized: true,
					unit: "dimensionless",
				},
			],
			metadata: [],
		} as any;

		categoryStore.state[symbol].add(measurement);
		categoryStore.setState((prev) => ({ ...prev }));

		const ring = categoryStore.state[symbol];
		expect(ring.isEmpty()).toBe(false);
		expect(ring.getLast()).toEqual(measurement);
		expect(ring.getLast()?.source).toBe("category");
	});

	it("integrates seamlessly with getMeasurementStore", () => {
		const symbol = "ETH/USD";
		const store = getMeasurementStore("category", symbol);
		expect(store).toBeDefined();

		const measurement: MeasurementT = {
			source: "category",
			symbol,
			tick: 101n,
			at: 1718000000000000000n,
			observedFrom: 1718000000000000000n,
			horizon: 0n,
			peerAt: 0n,
			peerObservedFrom: 0n,
			maturity: 1.0,
			snr: 0.9,
			snrDefined: true,
			metrics: [],
			metadata: [],
		} as any;

		store.add(measurement);
		expect(store.state.isEmpty()).toBe(false);
		expect(store.state.getLast()?.symbol).toBe("ETH/USD");
		expect(categoryStore.state[symbol]?.getLast()?.tick).toBe(101n);
	});
});
