// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import {
	beginHindsight,
	hindsightStore,
	ingestHindsightFrame,
} from "#/collections/hindsight";
import {
	doneFrame,
	excursionFrame,
	symbolFrame,
	tickFrame,
} from "#/collections/hindsight-frames";
import {
	bucketize,
	coordinateOf,
	fetchHindsightRuns,
	summariesOf,
	timelineOf,
} from "./hindsight-api";

const RUN = "1792187257188073000";

const stream = () => {
	beginHindsight(RUN, "BTC/USD");
	ingestHindsightFrame(
		excursionFrame(RUN, "BTC/USD", "up", {
			a_tick: 1,
			b_tick: 2,
			c_tick: 4,
			d_tick: 5,
			b_price: 100,
			c_price: 104,
			entry_tick: 2,
			entry_ask: 100.5,
			exit_tick: 4,
			exit_bid: 103.5,
		}),
	);
	ingestHindsightFrame(excursionFrame(RUN, "BTC/USD", "chop", {}));

	for (let tick = 1; tick <= 6; tick++) {
		ingestHindsightFrame(
			tickFrame(
				RUN,
				"BTC/USD",
				tick,
				99 + tick,
				tick === 3 ? Number.NaN : 101 + tick,
				100 + tick,
			),
		);
	}

	ingestHindsightFrame(doneFrame(RUN, "BTC/USD", 6));
	const loaded = hindsightStore.state.stream;
	if (loaded === null) throw new Error("no stream");
	return loaded;
};

describe("hindsight projection", () => {
	afterEach(() => {
		hindsightStore.setState(() => ({ index: null, stream: null }));
		vi.restoreAllMocks();
	});

	it("leaves a coordinate the tick does not define undefined, not zero", () => {
		const loaded = stream();
		expect(coordinateOf(loaded.ticks[0], "midpoint")).toBe(101);
		expect(coordinateOf(loaded.ticks[2], "midpoint")).toBeNull();
		expect(coordinateOf(loaded.ticks[2], "trade")).toBe(103);
	});

	it("keeps an empty slot's place so a later tick is not drawn in it", () => {
		const loaded = stream();
		const edge = loaded.ticks.filter(
			(tick) => tick.tick === 1 || tick.tick === 6,
		);
		const buckets = bucketize(edge, "midpoint", 3, 1, 6);

		expect(buckets.map((bucket) => bucket.index)).toEqual([0, 2]);
	});

	it("buckets ticks by tick number with OHLC over the ticks that define the coordinate", () => {
		const buckets = bucketize(stream().ticks, "midpoint", 3, 1, 6);
		expect(
			buckets.map((bucket) => [
				bucket.fromSequence,
				bucket.toSequence,
				bucket.observations,
			]),
		).toEqual([
			[1, 2, 2],
			[3, 4, 2],
			[5, 6, 2],
		]);
		expect(buckets[1].open).toBe(104);
		expect(buckets[1].close).toBe(104);
		expect(buckets[1].defined).toBe(true);
		expect(buckets[0].hasTouchDepth).toBe(false);
		expect(buckets[0].spreadFraction).toBeGreaterThan(0);
	});

	it("turns stored excursions into episodes, chop included", () => {
		const timeline = timelineOf(stream(), null, "midpoint", "time", 10, null);
		expect(timeline?.discovery.stored).toBe(2);
		expect(timeline?.discovery.episodes.map((episode) => episode.kind)).toEqual(
			["upward_excursion", "chop"],
		);

		const episode = timeline?.discovery.episodes.find(
			(item) => item.kind === "upward_excursion",
		);
		if (episode === undefined) throw new Error("no up episode");
		expect(episode.kind).toBe("upward_excursion");
		expect([episode.fromSequence, episode.toSequence]).toEqual([1, 5]);
		expect(episode.observedExcursion).toBeCloseTo(0.04);
		expect(
			episode.references.map((reference) => [
				reference.role,
				reference.capture.sequence,
				reference.value,
			]),
		).toEqual([
			["anchor", 2, 100.5],
			["shock_onset", 2, 100],
			["peak", 4, 104],
			["exit_anchor", 4, 103.5],
		]);
	});

	it("narrows to a window", () => {
		const timeline = timelineOf(stream(), null, "trade", "capture", 10, {
			from: 2,
			to: 4,
		});
		expect(timeline?.span.fromSequence).toBe(2);
		expect(timeline?.runSpan.toSequence).toBe(6);
		expect(
			timeline?.buckets.reduce((sum, bucket) => sum + bucket.observations, 0),
		).toBe(3);
	});

	it("summarizes the run index", () => {
		beginHindsight(RUN, "");
		ingestHindsightFrame(
			symbolFrame(RUN, "ETH/USD", {
				ticks: 9,
				touched: 8,
				up: 1,
				down: 2,
				top_excursion: -0.02,
			}),
		);
		ingestHindsightFrame(doneFrame(RUN, "", 9));

		const [summary] = summariesOf(hindsightStore.state.index);
		expect(summary.symbol).toBe("ETH/USD");
		expect(summary.defined).toBe(8);
		expect(summary.priceEpisodes).toBe(3);
		expect(summary.topExcursion).toBeCloseTo(0.02);
		expect(summary.topKind).toBe("downward_excursion");
	});

	it("reads runs with string ids and refuses a malformed one", async () => {
		const fetchMock = vi.fn().mockResolvedValueOnce(
			new Response(
				JSON.stringify([
					{
						id: RUN,
						startedAt: "2026-10-16T00:00:00Z",
						status: "recorded",
						trained: true,
						policies: [
							{
								epoch: "1792190000000000000",
								gate: true,
								surpriseZ: 1,
								warmup:
									"settled: error average at 0.15 no longer below its average at 0.01",
								learningRate: 0.01,
							},
						],
					},
				]),
			),
		);
		vi.stubGlobal("fetch", fetchMock);

		const runs = await fetchHindsightRuns();
		expect(runs).toEqual([
			{
				id: RUN,
				startedAt: "2026-10-16T00:00:00Z",
				status: "recorded",
				trained: true,
				policies: [
					{
						epoch: "1792190000000000000",
						gate: true,
						surpriseZ: 1,
						warmup:
							"settled: error average at 0.15 no longer below its average at 0.01",
						learningRate: 0.01,
					},
				],
				ticks: 0,
			},
		]);

		fetchMock.mockResolvedValueOnce(
			new Response(
				JSON.stringify([{ id: 1792187257188073000, startedAt: "x" }]),
			),
		);
		await expect(fetchHindsightRuns()).rejects.toThrow(/malformed run/);

		fetchMock.mockResolvedValueOnce(new Response("boom", { status: 500 }));
		await expect(fetchHindsightRuns()).rejects.toThrow(/500/);
		vi.unstubAllGlobals();
	});
});
