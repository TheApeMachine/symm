import { tableFromArrays, tableToIPC } from "apache-arrow";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	fetchHindsightRuns,
	fetchHindsightCaptures,
	fetchHindsightStates,
	fetchHindsightState,
	fetchHindsightEnvelope,
	fetchHindsightGaps,
	fetchHindsightLifecycle,
	fetchHindsightTimeline,
	fetchHindsightSymbols,
	fetchHindsightMetricMap,
	normalizeExcursionRecord,
	adaptMeasurementsToTimeline,
} from "./hindsight-api";

afterEach(() => vi.unstubAllGlobals());

const respond = (status: number, body: unknown) => {
	vi.stubGlobal("window", {
		location: { protocol: "http:", hostname: "localhost" },
	});
	const payload = typeof body === "string" ? body : JSON.stringify(body);
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(() =>
			Promise.resolve(new Response(payload, { status })),
		),
	);
};

describe("Hindsight archive reads", () => {
	it("preserves the run identity and timestamp used by run selection", async () => {
		const run = { id: "run", startedAt: "2026-09-09T00:00:00Z", positions: 2 };
		respond(200, [run]);
		expect(await fetchHindsightRuns()).toEqual([run]);
	});

	it("rejects storage-row JSON instead of rendering invalid dates and unselectable runs", async () => {
		respond(200, [{ ID: "run", StartedAt: "2026-09-09T00:00:00Z" }]);
		await expect(fetchHindsightRuns()).rejects.toThrow("invalid run metadata");
	});

	it.each([
		["runs", () => fetchHindsightRuns()],
		["captures", () => fetchHindsightCaptures("run")],
		["states", () => fetchHindsightStates("run")],
		["state", () => fetchHindsightState("run", 2, 1)],
		["envelope", () => fetchHindsightEnvelope("run", 2)],
		["gaps", () => fetchHindsightGaps("run")],
		["lifecycle", () => fetchHindsightLifecycle("run")],
		["symbols", () => fetchHindsightSymbols("run")],
		["semantics", () => fetchHindsightMetricMap()],
	])("surfaces a failed %s read instead of returning an empty archive", async (_name, read) => {
		respond(503, "archive unavailable");
		await expect(read()).rejects.toThrow("503");
	});

	it("surfaces a failed workbench timeline query instead of returning an empty archive", async () => {
		respond(503, "workbench service unavailable");
		await expect(fetchHindsightTimeline({ run: "run" })).rejects.toThrow(
			"workbench",
		);
	});

	it("decodes DuckDB Arrow IPC buckets from /workbench/query", async () => {
		vi.stubGlobal("window", {
			location: { protocol: "http:", hostname: "localhost" },
		});

		const listing = tableToIPC(
			tableFromArrays({
				database: ["symmtables", "symmtables"],
				schema: ["hindsight", "hindsight"],
				name: ["spot_ticker", "spot_trade"],
			}),
		);
		const buckets = tableToIPC(
			tableFromArrays({
				index: [0],
				from_sequence: [10],
				to_sequence: [10],
				from_at: ["2026-10-01T12:00:00.000Z"],
				to_at: ["2026-10-01T12:00:00.000Z"],
				tickers: [1],
				observations: [1],
				open: [101],
				high: [101],
				low: [101],
				close: [101],
				spread_fraction: [0.02],
				touch_depth: [3],
				trades: [0],
				trade_qty: [0],
			}),
		);

		const asBody = (bytes: Uint8Array): BodyInit =>
			bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;

		const fetcher = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
			const body = typeof init?.body === "string" ? init.body : "";
			if (body.includes("SHOW ALL TABLES")) {
				return new Response(asBody(listing), { status: 200 });
			}
			if (init?.method === "POST" && body.includes("bucket_idx")) {
				return new Response(asBody(buckets), { status: 200 });
			}
			if (_url.includes("/hindsight/excursions")) {
				return new Response("[]", { status: 200 });
			}
			return new Response("unexpected", { status: 500 });
		});
		vi.stubGlobal("fetch", fetcher);

		let progressCount = 0;
		const timeline = await fetchHindsightTimeline(
			{ run: "1", symbol: "BTC/USD" },
			{
				onProgress: (partial) => {
					progressCount++;
					expect(partial.symbol).toBe("BTC/USD");
				},
			},
		);

		expect(timeline).not.toBeNull();
		expect(timeline?.symbol).toBe("BTC/USD");
		expect(timeline?.totalObservations).toBe(1);
		expect(timeline?.buckets[0]?.open).toBe(101);
		expect(timeline?.measurements).toEqual([]);
		expect(progressCount).toBeGreaterThan(0);
	});

	it("keeps a genuinely absent exact artifact distinct from a failed read", async () => {
		respond(404, "not found");
		expect(await fetchHindsightState("run", 2, 1)).toBeNull();
		expect(await fetchHindsightEnvelope("run", 2)).toBeNull();
	});
});


describe("Hindsight excursion wire shape", () => {
	it("normalizes Go camelCase excursion JSON into episode geometry", () => {
		const record = normalizeExcursionRecord({
			epoch: 42,
			id: "exc-1",
			symbol: "BTC/USD",
			direction: "up",
			clearsFriction: true,
			precursorStartTick: 10,
			anchorTick: 20,
			extremumTick: 30,
			exitTick: 40,
			grossExcursion: 0.012,
			profitFraction: 0.008,
			observationCount: 12,
			entryPrice: 100,
			extremumPrice: 101.2,
			exitPrice: 100.8,
		});

		expect(record.precursorStartTick).toBe(10);
		expect(record.anchorTick).toBe(20);
		expect(record.exitTick).toBe(40);
		expect(record.clearsFriction).toBe(true);
		expect(record.grossExcursion).toBe(0.012);
	});

	it("accepts legacy snake_case fixtures without inventing ticks", () => {
		const record = normalizeExcursionRecord({
			epoch: 1,
			id: "legacy",
			symbol: "ETH/USD",
			direction: "down",
			clears_friction: false,
			precursor_start_tick: 5,
			anchor_tick: 6,
			exit_tick: 9,
			gross_excursion: 0.02,
		});

		expect(record.precursorStartTick).toBe(5);
		expect(record.exitTick).toBe(9);
		expect(record.clearsFriction).toBe(false);
	});

	it("maps up/down directions onto upward/downward episode kinds with A→C span", () => {
		const timeline = adaptMeasurementsToTimeline(
			[
				{
					label: "BTC/USD",
					symbol: "BTC/USD",
					source: "spot_ticker",
					seqIdx: 10,
					tick: 10,
					at: "2026-10-01T12:00:00.000Z",
					metrics: { bid: { raw: 100 }, ask: { raw: 102 } },
				} as any,
				{
					label: "BTC/USD",
					symbol: "BTC/USD",
					source: "spot_ticker",
					seqIdx: 40,
					tick: 40,
					at: "2026-10-01T12:01:00.000Z",
					metrics: { bid: { raw: 101 }, ask: { raw: 103 } },
				} as any,
			],
			{ run: "42", symbol: "BTC/USD", buckets: 2 },
			["BTC/USD"],
			[
				normalizeExcursionRecord({
					epoch: 42,
					id: "exc-up",
					symbol: "BTC/USD",
					direction: "up",
					clearsFriction: true,
					precursorStartTick: 10,
					anchorTick: 20,
					extremumTick: 30,
					exitTick: 40,
					grossExcursion: 0.01,
					profitFraction: 0.005,
					observationCount: 5,
					entryPrice: 101,
					extremumPrice: 102,
					exitPrice: 101.5,
				}),
			],
		);

		expect(timeline.discovery.episodes).toHaveLength(1);
		expect(timeline.discovery.episodes[0].kind).toBe("upward_excursion");
		expect(timeline.discovery.episodes[0].fromSequence).toBe(10);
		expect(timeline.discovery.episodes[0].toSequence).toBe(40);
		expect(timeline.discovery.episodes[0].references.some((r) => r.role === "shock_onset")).toBe(true);
		expect(timeline.discovery.episodes[0].references.every((r) => r.ordinal === 0)).toBe(true);
		expect(timeline.discovery.episodes[0].references.some((r) => r.role === "peak")).toBe(true);
		expect(timeline.buckets[0].open).toBe(101);
		expect(timeline.buckets[0].hasSpreadFraction).toBe(true);
	});

	it("counts websocket trades via metadata type for arrivals", () => {
		const timeline = adaptMeasurementsToTimeline(
			[
				{
					label: "BTC/USD",
					symbol: "BTC/USD",
					source: "websocket",
					seqIdx: 1,
					tick: 1,
					at: "2026-10-01T12:00:00.000Z",
					metrics: { bid: { raw: 100 }, ask: { raw: 102 }, bid_qty: { raw: 1 }, ask_qty: { raw: 2 } },
					peers: [
						{
							source: "websocket",
							seqIdx: 1,
							metrics: { qty: { raw: 0.5 } },
							metadata: { type: "trade" },
						} as any,
					],
				} as any,
			],
			{ run: "1", symbol: "BTC/USD", buckets: 1 },
		);
		expect(timeline.buckets[0].trades).toBe(1);
		expect(timeline.buckets[0].tradeQty).toBe(0.5);
		expect(timeline.buckets[0].hasTouchDepth).toBe(true);
		expect(timeline.buckets[0].touchDepth).toBe(3);
	});
});
