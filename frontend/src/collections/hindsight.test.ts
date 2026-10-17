import { afterEach, describe, expect, it, vi } from "vitest";
import {
	abortHindsight,
	beginHindsight,
	hindsightStore,
	ingestHindsightFrame,
} from "#/collections/hindsight";
import {
	doneFrame,
	errorFrame,
	excursionFrame,
	policyFrame,
	symbolFrame,
	tickFrame,
} from "#/collections/hindsight-frames";

const RUN = "1792187257188073000";

describe("hindsight stream store", () => {
	afterEach(() => {
		hindsightStore.setState(() => ({ index: null, stream: null }));
		vi.restoreAllMocks();
	});

	it("begins a request with the hub command, keeping the run a decimal string", () => {
		expect(beginHindsight(RUN, "BTC/USD")).toEqual({
			type: "hindsight",
			run: RUN,
			symbol: "BTC/USD",
		});
		expect(hindsightStore.state.stream?.status).toBe("streaming");
		expect(hindsightStore.state.index).toBeNull();
	});

	it("collects a symbol's frames, then completes on done", () => {
		beginHindsight(RUN, "BTC/USD");
		ingestHindsightFrame(policyFrame(RUN, "BTC/USD", "enter", "R01/R04", 0.01));
		ingestHindsightFrame(
			excursionFrame(RUN, "BTC/USD", "up", { b_price: 100, c_price: 103 }),
		);
		ingestHindsightFrame(tickFrame(RUN, "BTC/USD", 1, 99.5, 100.5, 101));
		ingestHindsightFrame(tickFrame(RUN, "BTC/USD", 2, 99.5, Number.NaN, 102));
		ingestHindsightFrame(doneFrame(RUN, "BTC/USD", 2));

		const stream = hindsightStore.state.stream;
		expect(stream?.status).toBe("done");
		expect(stream?.policies).toEqual([
			{
				action: "enter",
				path: "R01/R04",
				fields: {
					gain: 0.01,
					hold_seconds: 30,
					visits: 4,
					mean_reward: 0.01,
					confidence: 0.6,
				},
			},
		]);
		expect(stream?.excursions[0].kind).toBe("up");
		expect(stream?.excursions[0].fields.c_price).toBe(103);
		expect(stream?.ticks.map((tick) => tick.tick)).toEqual([1, 2]);
		expect(stream?.ticks[1].at).toBe(2000);
		expect(Number.isNaN(stream?.ticks[1].fields.ask)).toBe(true);
		expect(stream?.counts?.ticks).toBe(2);
	});

	it("routes the run index by its empty scope and reads the symbol from the key", () => {
		beginHindsight(RUN, "");
		ingestHindsightFrame(
			symbolFrame(RUN, "BTC/USD", { ticks: 3, top_excursion: 0.03 }),
		);
		ingestHindsightFrame(doneFrame(RUN, "", 3));

		expect(hindsightStore.state.index?.symbols).toHaveLength(1);
		expect(hindsightStore.state.index?.symbols[0].symbol).toBe("BTC/USD");
		expect(hindsightStore.state.index?.status).toBe("done");
	});

	it("does not apply frames of a request it no longer holds", () => {
		beginHindsight(RUN, "ETH/USD");
		ingestHindsightFrame(tickFrame(RUN, "BTC/USD", 1, 1, 2, 1.5));
		ingestHindsightFrame(
			tickFrame("1792100000000000000", "ETH/USD", 1, 1, 2, 1.5),
		);
		ingestHindsightFrame(doneFrame(RUN, "ETH/USD", 0));

		expect(hindsightStore.state.stream?.ticks).toEqual([]);
	});

	it("fails the stream on an error frame, a malformed frame or a closed socket", () => {
		const error = vi.spyOn(console, "error").mockImplementation(() => {});

		beginHindsight(RUN, "BTC/USD");
		ingestHindsightFrame(
			errorFrame(RUN, "BTC/USD", "no stored tick trading BTC/USD"),
		);
		expect(hindsightStore.state.stream?.status).toBe("error");
		expect(hindsightStore.state.stream?.error).toContain("no stored tick");

		beginHindsight(RUN, "BTC/USD");
		const broken = tickFrame(RUN, "BTC/USD", 1, 1, 2, 1.5);
		broken.layers[0].index = broken.layers[0].index.replace("/trades", "");
		ingestHindsightFrame(broken);
		ingestHindsightFrame(doneFrame(RUN, "BTC/USD", 1));
		expect(hindsightStore.state.stream?.status).toBe("error");
		expect(hindsightStore.state.stream?.error).toContain("malformed");

		beginHindsight(RUN, "");
		abortHindsight("hub socket closed");
		expect(hindsightStore.state.index?.error).toBe("hub socket closed");
		expect(error).toHaveBeenCalled();
	});
});
