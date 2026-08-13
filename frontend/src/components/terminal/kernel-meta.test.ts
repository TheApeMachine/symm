import { describe, expect, it } from "vitest";
import { sourceHeadlineMetric, sourceMetrics } from "./kernel-meta";

describe("sourceHeadlineMetric", () => {
	it("uses the dimensionless branching ratio as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe("metrics.spectral_radius");
	});

	it("uses classifier hypothesis separation as depthflow's headline", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe(
			"metrics.hypothesis_separation",
		);
		expect(sourceMetrics("depthflow")).toEqual([
			"hypothesis_separation",
			"loaded_score",
			"spoof_score",
			"thin_score",
			"neutral_score",
		]);
		expect(sourceMetrics("depthflow")).not.toContain("strength");
		expect(sourceMetrics("depthflow")).not.toContain("value");
	});

	it("uses classifier hypothesis separation as correlation's headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe(
			"metrics.hypothesis_separation",
		);
		expect(sourceMetrics("correlation")).toContain("hypothesis_separation");
		expect(sourceMetrics("correlation")).not.toContain("snr");
		expect(sourceMetrics("correlation")).not.toContain("strength");
		expect(sourceMetrics("correlation")).not.toContain("peak_score");
	});

	it("uses toxicity's normalized summary rather than raw touch size", () => {
		expect(sourceHeadlineMetric("toxicity")).toBe("metrics.strength");
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
