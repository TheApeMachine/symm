import { describe, expect, it } from "vitest";
import { createFrameCollection } from "./frames";

describe("createFrameCollection", () => {
	it("retains frames in a bounded circular buffer", () => {
		const store = createFrameCollection(3);

		store.actions.updateFrames([
			{ source: "tick", count: 1 },
			{ source: "tick", count: 2 },
			{ source: "tick", count: 3 },
			{ source: "tick", count: 4 },
		]);

		expect(store.state.frames.values()).toEqual([
			{ source: "tick", count: 2 },
			{ source: "tick", count: 3 },
			{ source: "tick", count: 4 },
		]);
		expect(store.state.frame).toEqual({ source: "tick", count: 4 });
	});

	it("indexes appended frames by symbol and source", () => {
		const store = createFrameCollection(2);

		store.actions.updateFrame({
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-07-14T12:00:00Z",
		});
		store.actions.updateFrame({
			source: "resonance",
			symbol: "ETH/USD",
			at: "2026-07-14T12:00:01Z",
		});

		expect(store.state.bySymbol["BTC/USD"]?.values()).toHaveLength(1);
		expect(store.state.bySource.resonance?.values()).toHaveLength(2);
	});
});
