import { describe, expect, it } from "vitest";
import { RingBuffer, signals } from "./app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";

describe("categoryStore", () => {
	const categoryStore = signals.category;

	it("initializes as an empty record of symbol ring buffers", () => {
		expect(categoryStore).toBeDefined();
		expect(typeof categoryStore.state).toBe("object");
	});

	it("stores MeasurementT objects per symbol ring buffer", () => {
		const symbol = "BTC/USD";
		if (!categoryStore.state[symbol]) {
			categoryStore.state[symbol] = new RingBuffer<MeasurementT>(50);
		}

		const measurement = new MeasurementT();
		measurement.source = "category";
		measurement.symbol = symbol;
		measurement.tick = 100n;
		measurement.at = 1718000000000000000n;
		measurement.observedFrom = 1718000000000000000n;
		measurement.maturity = 1.0;
		measurement.snr = 0.95;
		measurement.snrDefined = true;
		const m = new MetricT();
		m.name = "loaded_imbalance";
		m.raw = 0.85;
		m.normalized = 0.85;
		m.hasNormalized = true;
		measurement.metrics = [m];

		categoryStore.state[symbol].add(measurement);
		categoryStore.setState((prev) => ({ ...prev }));

		const ring = categoryStore.state[symbol];
		expect(ring.isEmpty()).toBe(false);
		expect(ring.getLast()).toEqual(measurement);
		expect(ring.getLast()?.source).toBe("category");
	});

	it("integrates seamlessly with signals.category", () => {
		const symbol = "ETH/USD";
		const store = signals.category;
		expect(store).toBeDefined();
		if (!store.state[symbol]) {
			store.state[symbol] = new RingBuffer<MeasurementT>(50);
		}

		const measurement = new MeasurementT();
		measurement.source = "category";
		measurement.symbol = symbol;
		measurement.tick = 101n;
		measurement.at = 1718000000000000000n;
		measurement.observedFrom = 1718000000000000000n;
		measurement.maturity = 1.0;
		measurement.snr = 0.9;
		measurement.snrDefined = true;

		store.state[symbol].add(measurement);
		expect(store.state[symbol].isEmpty()).toBe(false);
		expect(store.state[symbol].getLast()?.symbol).toBe("ETH/USD");
		expect(categoryStore.state[symbol]?.getLast()?.tick).toBe(101n);
	});
});
