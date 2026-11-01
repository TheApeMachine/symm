import { describe, expect, it } from "vitest";
import {
	kernelSparkPaths,
	sourceHeadline,
	sourceHeadlineMetric,
	sourceMetrics,
} from "./kernel-meta";

describe("sourceHeadline", () => {
	it("names the Map() field each live signal kernel leads with", () => {
		expect(sourceHeadline("depthflow")).toBe("loaded_score");
		expect(sourceHeadline("hawkes")).toBe("spectral_radius");
		expect(sourceHeadline("correlation")).toBe("herd_score");
		expect(sourceHeadline("liquidity")).toBe("scarcity_score");
	});

	it("returns null for sources with no named headline", () => {
		expect(sourceHeadline("resonance")).toBeNull();
		expect(sourceHeadline("manifold")).toBeNull();
	});
});

describe("sourceHeadlineMetric", () => {
	it("uses spectral radius as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe("metrics.spectral_radius");
	});

	it("uses metrics the live depthflow kernel publishes", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe("metrics.loaded_score");
		expect(sourceMetrics("depthflow")).toContain("loaded_score");
		expect(sourceMetrics("depthflow")).toContain("thin_score");
		expect(sourceMetrics("depthflow")).not.toContain("snr");
	});

	it("uses herd score as the correlation headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe("metrics.herd_score");
		expect(sourceMetrics("correlation")).toContain("herd_score");
		expect(sourceMetrics("correlation")).toContain("peak_score");
	});

	it("shows PumpDump's enter detector on the slate", () => {
		expect(sourceHeadline("pumpdump")).toBe("compression");
		expect(sourceMetrics("pumpdump")).toContain("compression");
		expect(sourceMetrics("pumpdump")).toContain("exhaustion");
		expect(sourceMetrics("pumpdump")).toContain("ignition");
	});

	it("lists SIGNALS.md Map keys on live signal kernels", () => {
		expect(sourceMetrics("cvd")).toContain("drive");
		expect(sourceMetrics("depthflow")).toContain("spoof_score");
		expect(sourceMetrics("hawkes")).toContain("maturity");
		expect(sourceMetrics("leadlag")).toContain("sync_score");
		expect(sourceMetrics("liquidity")).toContain("depth_score");
		expect(sourceMetrics("morphology")).toContain("bid_notional_lean");
	});

	it("supports every booter signal kernel in the inspector", () => {
		expect(sourceHeadlineMetric("toxicity")).toBe("metrics.support_score");
		expect(sourceHeadlineMetric("sentiment")).toBe("metrics.surge_score");
		expect(sourceHeadlineMetric("cvd")).toBe("metrics.drive");
	});
});

describe("kernelSparkPaths", () => {
	it("keeps existing points still when a new extreme arrives", () => {
		const before = kernelSparkPaths([0.2, 0.8]).spark;
		const after = kernelSparkPaths([0.2, 0.8, 5]).spark;

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
