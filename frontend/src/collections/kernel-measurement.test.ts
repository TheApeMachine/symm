import { describe, expect, it } from "vitest";
import { kernelMeasurement } from "#/collections/app";
import { Value } from "#/types/value";

describe("kernelMeasurement", () => {
	it("folds one kernel's layers off the aggregate tick for a symbol", () => {
		const aggregate = new Value({
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/100",
		});

		const layer = new TextEncoder().encode(JSON.stringify([0.42]));
		aggregate.layers.push(layer);
		aggregate.index.push(
			`${aggregate.prefix}/cvd/BTC/USD/signed_net_fraction`,
		);

		const reading = kernelMeasurement("cvd", "BTC/USD", aggregate);

		expect(reading?.scope).toBe("BTC/USD");
		expect(reading?.metric("signed_net_fraction")).toBe(0.42);
	});

	it("folds one multi-metric signal layer into named metrics", () => {
		const aggregate = new Value({
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/2",
		});
		const payload = [0.1, 0.2, 0.3, 1.5, 0.01, 0.8, 2.0];
		aggregate.layers.push(new TextEncoder().encode(JSON.stringify(payload)));
		aggregate.index.push(
			`${aggregate.prefix}/hawkes/BTC/USD/arrival_rate:buy/arrival_rate:sell/branching_spectral_radius/conditional_intensity/conditional_intensity_velocity/background_rate/excitation_decay`,
		);

		const reading = kernelMeasurement("hawkes", "BTC/USD", aggregate);

		expect(reading?.metric("conditional_intensity")).toBe(1.5);
		expect(reading?.metric("background_rate")).toBe(0.8);
		expect(reading?.metric("excitation_decay")).toBe(2.0);
	});
});
