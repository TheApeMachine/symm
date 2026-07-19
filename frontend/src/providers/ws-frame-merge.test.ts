import { describe, expect, it } from "vitest";
import { mergeFrameEntry, mergeFramePayload } from "./ws-frame-merge";

describe("mergeFramePayload", () => {
	it("replaces array snapshots with the latest payload", () => {
		const merged = mergeFramePayload(
			{
				holdings: [{ symbol: "BTC/USD", mark: 1 }],
			},
			{
				holdings: [{ symbol: "ETH/USD", mark: 2 }],
			},
		);

		expect(merged.holdings).toEqual([{ symbol: "ETH/USD", mark: 2 }]);
	});

	it("keeps only the latest analysis snapshots inside one paint window", () => {
		const merged = mergeFramePayload(
			{
				measurements: [{ symbol: "BTC/USD", at: "first" }],
				manifold: [{ symbol: "BTC/USD", epoch: 1 }],
			},
			{
				measurements: [{ symbol: "BTC/USD", at: "second" }],
				manifold: [{ symbol: "BTC/USD", epoch: 2 }],
			},
		);

		expect(merged.measurements).toEqual([{ symbol: "BTC/USD", at: "second" }]);
		expect(merged.manifold).toEqual([{ symbol: "BTC/USD", epoch: 2 }]);
	});

	it("preserves ordered findings inside one paint window", () => {
		const merged = mergeFramePayload(
			{ findings: [{ id: "first" }] },
			{ findings: [{ id: "second" }] },
		);

		expect(merged.findings).toEqual([{ id: "first" }, { id: "second" }]);
	});

	it("shallow-merges object maps inside one worker window", () => {
		const merged = mergeFramePayload(
			{
				tick: { count: 1, open: 0 },
				lifecycle: { "BTC/USD": "managing" },
			},
			{
				tick: { candidates: 4 },
				lifecycle: { "ETH/USD": "shaped" },
			},
		);

		expect(merged.tick).toEqual({ count: 1, open: 0, candidates: 4 });
		expect(merged.lifecycle).toEqual({
			"BTC/USD": "managing",
			"ETH/USD": "shaped",
		});
	});

	it("keeps the latest scalar frame", () => {
		expect(mergeFrameEntry("old", "new")).toBe("new");
	});
});
