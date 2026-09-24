import * as flatbuffers from "flatbuffers";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import {
	fetchHindsightCaptures,
	fetchHindsightEnvelope,
	fetchHindsightGaps,
	fetchHindsightLifecycle,
	fetchHindsightMetricMap,
	fetchHindsightRuns,
	fetchHindsightState,
	fetchHindsightStates,
	fetchHindsightSymbols,
	fetchHindsightTimeline,
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
