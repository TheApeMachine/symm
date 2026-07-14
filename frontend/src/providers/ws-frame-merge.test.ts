import { describe, expect, it } from "vitest";
import { mergeFrameEntry, mergeFramePayload } from "./ws-frame-merge";

describe("mergeFramePayload", () => {
	it("replaces array snapshots with the latest payload", () => {
		const merged = mergeFramePayload(
			{
				positions: [{ symbol: "BTC/USD", mark: 1 }],
			},
			{
				positions: [{ symbol: "ETH/USD", mark: 2 }],
			},
		);

		expect(merged.positions).toEqual([{ symbol: "ETH/USD", mark: 2 }]);
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
