import { describe, expect, it } from "vitest";
import {
	compactNumber,
	crossSectionReadoutFromFrame,
} from "./cross-section-panel";

describe("crossSectionReadoutFromFrame", () => {
	it("reduces a backend CrossSectionSummary frame into display-ready fields", () => {
		const readout = crossSectionReadoutFromFrame({
			symbols: ["BTC/USD", "ETH/USD", "SOL/USD"],
			leader: "BTC/USD",
			leadershipThreshold: 0.018,
			breadth: 0.6667,
			volumes: [10, 20, 30],
			quoteNotionals: [1000, 3000, 2000],
			executableDepths: [5, 15, 10],
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

	it("ignores non-numeric entries mixed into the arrays", () => {
		const readout = crossSectionReadoutFromFrame({
			symbols: ["BTC/USD", 42, null],
			volumes: [10, Number.NaN, "20", 30],
		});

		expect(readout.symbolCount).toBe(1);
		expect(readout.medianVolume).toBe(20);
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
