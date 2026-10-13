import { describe, expect, it } from "vitest";
import {
	kernelSparkPaths,
	sourceHeadline,
	sourceHeadlineMetric,
	sourceMetrics,
} from "./kernel-meta";

describe("sourceHeadline", () => {
	it("names the metric each signal kernel leads with", () => {
		expect(sourceHeadline("depthflow")).toBe("book_imbalance");
		expect(sourceHeadline("hawkes")).toBe("branching_spectral_radius");
		expect(sourceHeadline("correlation")).toBe("cohort_covariance_score");
		expect(sourceHeadline("cvd")).toBe("signed_net_fraction");
		expect(sourceHeadline("toxicity")).toBe("retreat_fraction:bid");
	});

	it("returns null for sources with no named headline", () => {
		expect(sourceHeadline("resonance")).toBeNull();
		expect(sourceHeadline("manifold")).toBeNull();
	});
});

describe("sourceHeadlineMetric", () => {
	it("uses the dimensionless branching ratio as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe(
			"metrics.branching_spectral_radius",
		);
	});

	it("uses a metric the live depthflow measurement publishes", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe("metrics.book_imbalance");
		expect(sourceMetrics("depthflow")).toContain("book_imbalance");
		expect(sourceMetrics("depthflow")).toContain("touch_notional:bid");
		expect(sourceMetrics("depthflow")[0]).toBe("snr");
	});

	it("uses the live correlation hypothesis as its headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe(
			"metrics.cohort_covariance_score",
		);
		expect(sourceMetrics("correlation")).toContain("cohort_covariance_score");
		expect(sourceMetrics("correlation")[0]).toBe("snr");
	});

	it("uses the anchor evidence names emitted by leadlag", () => {
		expect(sourceHeadlineMetric("leadlag")).toBe(
			"metrics.led_peer_share",
		);
		expect(sourceMetrics("leadlag")).toContain("led_peer_share");
		expect(sourceMetrics("leadlag")).toContain("best_lag_seconds_median");
		expect(sourceMetrics("leadlag")[0]).toBe("snr");
	});

	it("shows PumpDump's measured spread contract", () => {
		expect(sourceMetrics("pumpdump")).toContain("spread_ratio");
		expect(sourceMetrics("pumpdump")).toContain("volume_rate");
	});

	it("uses toxicity's emitted retreat fraction rather than an absent summary", () => {
		expect(sourceHeadlineMetric("toxicity")).toBe(
			"metrics.retreat_fraction:bid",
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

describe("kernelSparkPaths", () => {
	it("keeps existing points still when a new extreme arrives", () => {
		const before = kernelSparkPaths([0.2, 0.8]).spark;
		const after = kernelSparkPaths([0.2, 0.8, 5]).spark;

		// The leftmost (oldest) point keeps its exact coordinate: a new extreme
		// clamps to the rail instead of renormalizing the whole window.
		expect(after.split(" ")[0]).toBe(before.split(" ")[0]);
		expect(before).toBe("0.0,23.8 150.0,8.2");
		expect(after).toBe("0.0,23.8 75.0,8.2 150.0,3.0");
	});

	it("folds signed readings into the lower half instead of renormalizing", () => {
		expect(kernelSparkPaths([-1, 0, 1]).spark).toBe(
			"0.0,29.0 75.0,29.0 150.0,3.0",
		);
	});

	it("clamps out-of-domain raw readings to the rails", () => {
		expect(kernelSparkPaths([2, 4]).spark).toBe("0.0,3.0 150.0,3.0");
		expect(kernelSparkPaths([-3, -2]).spark).toBe("0.0,29.0 150.0,29.0");
	});
});
