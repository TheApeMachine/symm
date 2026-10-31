import { describe, expect, it } from "vitest";
import { mergeWorkspaceAggregate, preserveLitLayers, Value } from "./value";

describe("mergeWorkspaceAggregate", () => {
	it("unions layers on the same tick", () => {
		const prefix = "workspace/tick/aggregate/1/99";
		const rich = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			tick: 99n,
			index: [
				`${prefix}/cvd/BTC/USD/midpoint_log_return`,
				`${prefix}/train/regions/BTC/USD`,
			],
			layers: [
				new TextEncoder().encode(JSON.stringify([0.42])),
				new TextEncoder().encode(JSON.stringify([])),
			],
		});
		const sparse = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			tick: 99n,
			index: [`${prefix}/spot:trade`],
			layers: [new TextEncoder().encode(JSON.stringify([]))],
		});

		const merged = mergeWorkspaceAggregate(rich, sparse);
		expect(merged.index.length).toBe(3);
	});

	it("replaces the aggregate on a new tick", () => {
		const prior = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/10",
			tick: 10n,
			index: ["workspace/tick/aggregate/0/10/cvd/BTC/USD/signed_net_fraction"],
			layers: [new TextEncoder().encode(JSON.stringify([-0.2]))],
		});
		const incoming = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/11",
			tick: 11n,
			index: ["workspace/tick/aggregate/0/11/spot:trade"],
			layers: [new TextEncoder().encode(JSON.stringify([]))],
		});

		const merged = mergeWorkspaceAggregate(prior, incoming);
		expect(merged.tick).toBe(11n);
		expect(merged.index.length).toBe(1);
		expect(merged.index[0]).toContain("spot:trade");
	});
});

describe("preserveLitLayers", () => {
	it("keeps prior non-empty active when the next training step carries []", () => {
		const lit = [{ key: 2, brightness: 0.8 }];
		const prior = new Value({
			origin: "training",
			role: "step",
			scope: "BTC/USD",
			prefix: "training/step/BTC/USD/1/10",
			tick: 10n,
			index: ["training/step/BTC/USD/1/10/active"],
			layers: [new TextEncoder().encode(JSON.stringify(lit))],
		});
		const incoming = new Value({
			origin: "training",
			role: "step",
			scope: "BTC/USD",
			prefix: "training/step/BTC/USD/1/11",
			tick: 11n,
			index: [
				"training/step/BTC/USD/1/11/active",
				"training/step/BTC/USD/1/11/gate",
			],
			layers: [
				new TextEncoder().encode(JSON.stringify([])),
				new TextEncoder().encode(JSON.stringify({ passed: false })),
			],
		});

		const kept = preserveLitLayers(prior, incoming);
		expect(kept.reading("active")).toEqual(lit);
		expect(kept.reading("gate")).toEqual({ passed: false });
	});
});
