import { describe, expect, it } from "vitest";
import {
	kernelSparkPaths,
	sourceHeadline,
	sourceHeadlineMetric,
	sourceMetrics,
} from "./kernel-meta";

describe("sourceHeadline", () => {
	it("names the metric each live signal kernel leads with", () => {
		expect(sourceHeadline("depthflow")).toBe("bid_ladder_build");
		expect(sourceHeadline("hawkes")).toBe("supercritical_cascade");
		expect(sourceHeadline("correlation")).toBe("structural_alignment");
		expect(sourceHeadline("liquidity")).toBe("spread_expansion");
	});

	it("returns null for sources with no named headline", () => {
		expect(sourceHeadline("resonance")).toBeNull();
		expect(sourceHeadline("manifold")).toBeNull();
	});
});

describe("sourceHeadlineMetric", () => {
	it("uses supercritical cascade as Hawkes' headline", () => {
		expect(sourceHeadlineMetric("hawkes")).toBe(
			"metrics.supercritical_cascade",
		);
	});

	it("uses metrics the live depthflow kernel publishes", () => {
		expect(sourceHeadlineMetric("depthflow")).toBe(
			"metrics.bid_ladder_build",
		);
		expect(sourceMetrics("depthflow")).toContain("bid_ladder_build");
		expect(sourceMetrics("depthflow")).toContain("bid_ladder_release");
		expect(sourceMetrics("depthflow")).toContain("ask_ladder_release");
		expect(sourceMetrics("depthflow")).not.toContain("snr");
	});

	it("uses board structural alignment as the correlation headline", () => {
		expect(sourceHeadlineMetric("correlation")).toBe(
			"metrics.structural_alignment",
		);
		expect(sourceMetrics("correlation")).toContain("structural_alignment");
		expect(sourceMetrics("correlation")).not.toContain("snr");
	});

	it("shows PumpDump's enter detector on the slate", () => {
		expect(sourceHeadline("pumpdump")).toBe("coiled_compression");
		expect(sourceMetrics("pumpdump")).toContain("coiled_compression");
		expect(sourceMetrics("pumpdump")).toContain("rejection_tick");
		expect(sourceMetrics("pumpdump")).toContain("board_coiled");
	});

	it("rejects dormant or unsupported sources", () => {
		expect(() => sourceHeadlineMetric("toxicity")).toThrow(
			"unsupported measurement source: toxicity",
		);
		expect(() => sourceHeadlineMetric("sentiment")).toThrow(
			"unsupported measurement source: sentiment",
		);
		expect(() => sourceMetrics("cvd")).toThrow(
			"unsupported measurement source: cvd",
		);
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
