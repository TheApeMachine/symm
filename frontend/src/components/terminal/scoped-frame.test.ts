import { describe, expect, it } from "vitest";
import { resolveScopedFrame } from "./scoped-frame";

describe("resolveScopedFrame", () => {
	it("TestFrontendConcreteFocusNeverBorrowsMeasurementFrame", () => {
		const scoped = resolveScopedFrame(
			{
				"ETH/USD": {
					role: "measurement",
					source: "fluid",
					scope: "ETH/USD",
				},
			},
			"BTC/USD",
			"fluid",
		);

		expect(scoped).toMatchObject({
			frame: null,
			mode: "missing",
			sourceName: "fluid",
			symbol: "BTC/USD",
		});
	});

	it("TestFrontendConcreteFocusNeverBorrowsResonanceFrame", () => {
		const scoped = resolveScopedFrame(
			{
				frame: {
					role: "resonance",
					focus: { symbol: "ETH/USD", layers: [{ state: [1, 2] }] },
				},
				frames: [],
			},
			"BTC/USD",
			"resonance",
		);

		expect(scoped.frame).toBeNull();
		expect(scoped.mode).toBe("missing");
	});

	it("TestFrontendConcreteFocusNeverBorrowsCognitiveFrame", () => {
		const scoped = resolveScopedFrame(
			{
				"ETH/USD": {
					role: "cognitive",
					scope: "ETH/USD",
				},
			},
			"BTC/USD",
			"cognitive",
		);

		expect(scoped.frame).toBeNull();
		expect(scoped.mode).toBe("missing");
	});

	it("TestFrontendStreamPreviewCanUseFirstLiveFrame", () => {
		const scoped = resolveScopedFrame(
			{
				frame: {
					role: "resonance",
					focus: { symbol: "ETH/USD", layers: [{ state: [1, 2] }] },
				},
			},
			"stream",
			"resonance",
		);

		expect(scoped.mode).toBe("stream_preview");
		expect(scoped.frame).toMatchObject({ symbol: "ETH/USD" });
	});
});
