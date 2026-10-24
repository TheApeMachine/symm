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
});
