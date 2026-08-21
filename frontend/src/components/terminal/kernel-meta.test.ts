import { describe, expect, it } from "vitest";
import { sourceHeadlineMetric, sourceMetrics } from "./kernel-meta";

describe("sourceHeadlineMetric", () => {
	it("uses the dimensionless branching ratio as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe("metrics.spectral_radius");
	});

	it("uses a metric the live depthflow measurement publishes", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe("metrics.loaded_score");
		expect(sourceMetrics("depthflow")).toEqual([
			"touch_imbalance",
			"deep_imbalance",
			"loaded_score",
			"spoof_score",
			"thin_score",
		]);
	});

	it("uses the live correlation hypothesis as its headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe("metrics.herd_score");
		expect(sourceMetrics("correlation")).toContain("herd_score");
	});

	it("uses the anchor evidence names emitted by leadlag", () => {
		expect(sourceHeadlineMetric("leadlag")).toBe("metrics.inefficient");
		expect(sourceMetrics("leadlag")).toContain("sync");
	});

	it("shows PumpDump's complete side-aware measurement contract", () => {
		expect(sourceMetrics("pumpdump")).toEqual([
			"hypothesis_separation",
			"best_price:buy",
			"best_price:sell",
			"midpoint",
			"trade_price",
			"trade_quantity",
			"rvol",
			"spread",
			"compression",
			"precursor:buy",
			"precursor:sell",
			"exhaustion:buy",
			"exhaustion:sell",
		]);
	});

	it("uses toxicity's emitted intensity rather than an absent summary", () => {
		expect(sourceHeadlineMetric("toxicity")).toBe(
			"metrics.toxicity_intensity",
		);
	});

	it("rejects unsupported sources instead of inventing a metric fallback", () => {
		expect(() => sourceHeadlineMetric("unknown")).toThrow(
			"unsupported measurement source: unknown",
		);
		expect(() => sourceMetrics("unknown")).toThrow(
			"unsupported measurement source: unknown",
		);
	});
});
