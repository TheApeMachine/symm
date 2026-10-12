// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { keysHash, tokensOf } from "./cognitive-tree";

describe("tokensOf", () => {
	it("rebuilds stored-key tokens from path_token metrics", () => {
		const m = new MeasurementT();
		m.metrics = Object.entries({ "path_token:00:0": 1, "path_token:00:1": 2, "path_token:01:0": 11 }).map(([name, raw]) => {
			const metric = new MetricT();
			metric.name = name;
			metric.raw = raw;
			return metric;
		});
		expect(tokensOf(m)).toEqual(["R01-R02", "R11"]);
	});
});

describe("keysHash", () => {
	it("matches strategy.keysHash (FNV-1a 32)", () => {
		expect(keysHash(["a", "b"])).toBe(0x28e4c710);
	});
});
