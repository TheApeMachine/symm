import { describe, expect, it } from "vitest";
import { sourceHeadlineMetric, sourceMetrics } from "./kernel-meta";

describe("sourceHeadlineMetric", () => {
	it("uses the dimensionless branching ratio as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe("metrics.spectral_radius");
	});

	it("uses signal-to-noise separation as depthflow's headline", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe("metrics.snr");
		expect(sourceMetrics("depthflow")).toEqual([
			"snr",
			"loaded_score",
			"spoof_score",
			"thin_score",
			"neutral_score",
		]);
		expect(sourceMetrics("depthflow")).not.toContain("strength");
		expect(sourceMetrics("depthflow")).not.toContain("value");
	});

	it("uses signal-to-noise separation as correlation's headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe("metrics.snr");
		expect(sourceMetrics("correlation")).toContain("snr");
		expect(sourceMetrics("correlation")).not.toContain("strength");
		expect(sourceMetrics("correlation")).not.toContain("peak_score");
	});
});
