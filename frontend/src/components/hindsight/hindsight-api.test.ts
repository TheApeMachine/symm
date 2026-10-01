import * as flatbuffers from "flatbuffers";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { Measurement } from "#/providers/telemetry/telemetry/measurement";
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
	vi.stubGlobal(
		"fetch",
		vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })),
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

	it("surfaces a failed timeline WebSocket connection instead of returning an empty archive", async () => {
		respond(200, []);
		vi.stubGlobal(
			"WebSocket",
			class {
				onerror: (() => void) | null = null;

				constructor() {
					queueMicrotask(() => this.onerror?.());
				}
			},
		);
		await expect(fetchHindsightTimeline({ run: "run" })).rejects.toThrow(
			"WebSocket",
		);
	});

	it("streams and decodes FlatBuffers MeasurementsFrame over WebSocket", async () => {
		respond(200, []);

		const builder = new flatbuffers.Builder(1024);
		const source = builder.createString("spot_ticker");
		const symbol = builder.createString("BTC/USD");
		Measurement.startMeasurement(builder);
		Measurement.addSource(builder, source);
		Measurement.addSymbol(builder, symbol);
		Measurement.addTick(builder, 10n);
		Measurement.addAt(builder, 1000000000000n);
		const measurementOffset = Measurement.endMeasurement(builder);

		const rowsOffset = MeasurementsFrame.createRowsVector(builder, [
			measurementOffset,
		]);
		MeasurementsFrame.startMeasurementsFrame(builder);
		MeasurementsFrame.addRows(builder, rowsOffset);
		const frameOffset = MeasurementsFrame.endMeasurementsFrame(builder);
		builder.finish(frameOffset);
		const bytes = builder.asUint8Array().slice();

		class MockWebSocket {
			binaryType = "blob";
			onmessage: ((event: MessageEvent) => void) | null = null;
			onclose: (() => void) | null = null;
			onerror: (() => void) | null = null;

			constructor() {
				setTimeout(() => {
					if (this.onmessage) {
						this.onmessage({ data: bytes.buffer } as MessageEvent);
					}
					if (this.onclose) {
						this.onclose();
					}
				}, 5);
			}

			close() {}
		}

		vi.stubGlobal("WebSocket", MockWebSocket);

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
