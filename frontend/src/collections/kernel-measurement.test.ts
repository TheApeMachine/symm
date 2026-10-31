import { describe, expect, it } from "vitest";
import { ensureSignalStore, latest, signals } from "#/collections/app";
import { ingest } from "#/providers/websocket";
import { Value } from "#/types/value";

describe("latest signal frames", () => {
	it("reads a kernel frame from its origin store by scope", () => {
		const store = ensureSignalStore("cvd");
		store.setState(() => ({}));

		const frame = new Value({
			origin: "cvd",
			role: "metric",
			scope: "BTC/USD",
			prefix: "cvd/metric/BTC/USD",
			tick: 1n,
		});
		const layer = new TextEncoder().encode(JSON.stringify([0.42]));
		frame.layers.push(layer);
		frame.index.push(`${frame.prefix}/signed_net_fraction`);

		ingest([frame]);

		expect(latest("cvd", "BTC/USD")?.metric("signed_net_fraction")).toBe(0.42);
		expect(latest("cvd", "BTC-USD")?.metric("signed_net_fraction")).toBe(0.42);
	});

	it("expands multi-metric layers on Value.metrics", () => {
		const frame = new Value({
			origin: "hawkes",
			role: "metric",
			scope: "BTC/USD",
			prefix: "hawkes/metric/BTC/USD",
			tick: 2n,
		});
		const payload = [0.1, 0.2, 0.3, 1.5, 0.01, 0.8, 2.0];
		frame.layers.push(new TextEncoder().encode(JSON.stringify(payload)));
		frame.index.push(
			`${frame.prefix}/arrival_rate:buy/arrival_rate:sell/branching_spectral_radius/conditional_intensity/conditional_intensity_velocity/background_rate/excitation_decay`,
		);

		ensureSignalStore("hawkes").setState(() => ({ "BTC/USD": frame }));

		const metrics = latest("hawkes", "BTC/USD")?.metrics() ?? {};
		expect(metrics.conditional_intensity).toBe(1.5);
		expect(metrics.background_rate).toBe(0.8);
		expect(metrics.excitation_decay).toBe(2.0);
	});

	it("merges same-tick layers for one origin/scope", () => {
		ensureSignalStore("liquidity").setState(() => ({}));

		const prefix = "liquidity/metric/aggregate";
		const first = new Value({
			origin: "liquidity",
			role: "metric",
			scope: "aggregate",
			prefix,
			tick: 9n,
		});
		first.layers.push(new TextEncoder().encode(JSON.stringify([0.01])));
		first.index.push(`${prefix}/market_friction`);

		const second = new Value({
			origin: "liquidity",
			role: "metric",
			scope: "aggregate",
			prefix,
			tick: 9n,
		});
		second.layers.push(new TextEncoder().encode(JSON.stringify([0.02])));
		second.index.push(`${prefix}/capacity_stress`);

		ingest([first, second]);

		const metrics = latest("liquidity", "aggregate")?.metrics() ?? {};
		expect(metrics.market_friction).toBe(0.01);
		expect(metrics.capacity_stress).toBe(0.02);
	});

	it("clears between cases via store reset", () => {
		signals.cvd?.setState(() => ({}));
		expect(latest("cvd", "BTC/USD")).toBeUndefined();
	});
});
