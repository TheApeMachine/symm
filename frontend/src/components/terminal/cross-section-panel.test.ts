import { describe, expect, it } from "vitest";
import {
	compactNumber,
	crossSectionReadoutFromFrame,
	crossSectionReadoutFromMeasurements,
	retainCrossSectionReadout,
} from "./cross-section-panel";

describe("crossSectionReadoutFromFrame", () => {
	it("reduces a backend CrossSectionSummary frame into display-ready fields", () => {
		const readout = crossSectionReadoutFromFrame({
			metrics: [
				{
					symbol: "BTC/USD",
					volume: 10,
					quoteNotional: 1000,
					executableDepth: 5,
				},
				{
					symbol: "ETH/USD",
					volume: 20,
					quoteNotional: 3000,
					executableDepth: 15,
				},
				{
					symbol: "SOL/USD",
					volume: 30,
					quoteNotional: 2000,
					executableDepth: 10,
				},
			],
			leader: "BTC/USD",
			leadershipThreshold: 0.018,
			breadth: 0.6667,
		});

		expect(readout.leader).toBe("BTC/USD");
		expect(readout.symbolCount).toBe(3);
		expect(readout.leadershipThresholdPercent).toBeCloseTo(1.8);
		expect(readout.breadthPercent).toBeCloseTo(66.67, 2);
		expect(readout.medianVolume).toBe(20);
		expect(readout.medianQuoteNotional).toBe(2000);
		expect(readout.medianExecutableDepth).toBe(10);
	});

	it("falls back to zeroed fields when there is no frame yet", () => {
		const readout = crossSectionReadoutFromFrame(null);

		expect(readout.leader).toBe("");
		expect(readout.symbolCount).toBe(0);
		expect(readout.leadershipThresholdPercent).toBe(0);
		expect(readout.breadthPercent).toBe(0);
		expect(readout.medianVolume).toBe(0);
	});

	it("ignores malformed metric rows while keeping valid peers", () => {
		const readout = crossSectionReadoutFromFrame({
			metrics: [
				{ symbol: "BTC/USD", volume: 10 },
				{ symbol: "", volume: 99 },
				{ volume: 40 },
				{ symbol: "ETH/USD", volume: 30 },
			],
		});

		expect(readout.symbolCount).toBe(2);
		expect(readout.medianVolume).toBe(20);
	});

	it("still supports legacy flat-array diagnostics frames", () => {
		const readout = crossSectionReadoutFromFrame({
			symbols: ["BTC/USD", "ETH/USD"],
			volumes: [10, 30],
			quoteNotionals: [1000, 3000],
			executableDepths: [5, 15],
		});

		expect(readout.symbolCount).toBe(2);
		expect(readout.medianVolume).toBe(20);
		expect(readout.medianQuoteNotional).toBe(2000);
		expect(readout.medianExecutableDepth).toBe(10);
	});
});

describe("crossSectionReadoutFromMeasurements", () => {
	it("derives peer depth from current backend measurement rows", () => {
		const readout = crossSectionReadoutFromMeasurements([
			{
				source: "liquidity",
				symbol: "BTC/USD",
				at: "1",
				validity: { state: "valid", readiness: "observation" },
				scale: { kind: "window", from: "1", through: "1" },
				metrics: {
					executable_touch_depth: { raw: 5 },
					reported_volume_notional: { raw: 1000 },
				},
			},
			{
				source: "liquidity",
				symbol: "ETH/USD",
				at: "1",
				validity: { state: "valid", readiness: "observation" },
				scale: { kind: "window", from: "1", through: "1" },
				metrics: {
					executable_touch_depth: { raw: 15 },
					reported_volume_notional: { raw: 3000 },
				},
			},
		]);

		expect(readout.symbolCount).toBe(2);
		expect(readout.leader).toBe("ETH/USD");
		expect(readout.medianExecutableDepth).toBe(10);
		expect(readout.medianQuoteNotional).toBe(2000);
	});

	it("keeps per-source rows for the same symbol instead of replacing metrics", () => {
		const readout = crossSectionReadoutFromMeasurements([
			{
				source: "correlation",
				symbol: "BTC/USD",
				at: "1",
				validity: { state: "valid", readiness: "observation" },
				scale: { kind: "window", from: "1", through: "1" },
				metrics: { strength: { raw: 1 } },
			},
			{
				source: "liquidity",
				symbol: "BTC/USD",
				at: "1",
				validity: { state: "valid", readiness: "observation" },
				scale: { kind: "window", from: "1", through: "1" },
				metrics: {
					executable_touch_depth: { raw: 15 },
					reported_volume_notional: { raw: 3000 },
				},
			},
		]);

		expect(readout.symbolCount).toBe(1);
		expect(readout.leader).toBe("BTC/USD");
		expect(readout.medianExecutableDepth).toBe(15);
		expect(readout.medianQuoteNotional).toBe(3000);
	});
});

describe("retainCrossSectionReadout", () => {
	const populated = crossSectionReadoutFromFrame({
		metrics: [
			{
				symbol: "BTC/USD",
				volume: 10,
				quoteNotional: 1000,
				executableDepth: 5,
			},
			{
				symbol: "ETH/USD",
				volume: 30,
				quoteNotional: 3000,
				executableDepth: 15,
			},
		],
		leader: "BTC/USD",
		leadershipThreshold: 0.02,
		breadth: 0.5,
	});

	it("keeps the previous snapshot when the next frame has no peer metrics", () => {
		const empty = crossSectionReadoutFromFrame({
			metrics: [],
			leader: "",
			leadershipThreshold: 0,
			breadth: 0,
		});

		expect(retainCrossSectionReadout(populated, empty)).toEqual(populated);
	});

	it("accepts the first populated frame after an empty startup frame", () => {
		const empty = crossSectionReadoutFromFrame(null);

		expect(retainCrossSectionReadout(null, empty).symbolCount).toBe(0);
		expect(retainCrossSectionReadout(empty, populated)).toEqual(populated);
	});
});

describe("compactNumber", () => {
	it("renders millions with an M suffix", () => {
		expect(compactNumber(2_500_000)).toBe("2.50M");
	});

	it("renders thousands with a K suffix", () => {
		expect(compactNumber(4_200)).toBe("4.2K");
	});

	it("renders sub-thousand values with fixed precision", () => {
		expect(compactNumber(12.3456)).toBe("12.35");
		expect(compactNumber(0.1234)).toBe("0.1234");
	});
});
