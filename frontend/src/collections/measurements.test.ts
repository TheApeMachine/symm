import { beforeEach, describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import { tickStore } from "#/collections/tick";
import { applyFramePayload } from "#/providers/ws-stores";
import type { Measurement } from "#/types/measurement";
import {
	flattenMeasurementBuffer,
	headlineSeriesFromBuffer,
	latestMeasurementReadings,
	latestPublishedStamp,
	measurementGroupKey,
	measurementsStore,
	measurementTickCount,
} from "./measurements";

const measurement = (metric: string, at: string, raw: number): Measurement => ({
	source: "leadlag",
	metric,
	symbol: "BTC/USD",
	stream: "lead_lag",
	at,
	raw,
	normalized: raw,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: at, through: at },
});

describe("measurementsStore", () => {
	beforeEach(() => {
		measurementsStore.setState(() => ({
			measurements: {},
			version: 0,
		}));
		tickStore.setState(() => {
			const frames = Circular<Record<string, unknown>>(256);

			return {
				frame: null,
				frames,
				history: frames,
				bySymbol: {},
				bySource: {},
			};
		});
	});

	it("retains one circular-buffer slot per observation tick", () => {
		measurementsStore.actions.updateFrame([
			measurement("strength", "2026-07-14T16:25:24Z", 0.11),
			measurement("decoupled", "2026-07-14T16:25:24Z", 0.11),
			measurement("correlation", "2026-07-14T16:25:24Z", 0.43),
		]);

		const buffer = measurementsStore.state.measurements["BTC/USD"]?.leadlag;

		expect(measurementTickCount(buffer)).toBe(1);
		expect(latestMeasurementReadings(buffer)).toHaveLength(3);
		expect(flattenMeasurementBuffer(buffer)).toHaveLength(3);
		expect(measurementsStore.state.version).toBe(1);
	});

	it("pushes a new slot on every publish frame even when the timestamp is unchanged", () => {
		measurementsStore.actions.updateFrame([
			measurement("strength", "2026-07-14T16:25:24Z", 0.11),
			measurement("decoupled", "2026-07-14T16:25:24Z", 0.11),
		]);
		measurementsStore.actions.updateFrame([
			measurement("correlation", "2026-07-14T16:25:24Z", 0.43),
		]);

		const buffer = measurementsStore.state.measurements["BTC/USD"]?.leadlag;

		expect(measurementTickCount(buffer)).toBe(2);
		expect(latestMeasurementReadings(buffer)).toHaveLength(1);
		expect(headlineSeriesFromBuffer(buffer, "strength")).toEqual([0.11]);
		expect(headlineSeriesFromBuffer(buffer, "correlation")).toEqual([0.43]);
	});

	it("pushes a new slot when the observation timestamp advances", () => {
		measurementsStore.actions.updateFrame([
			measurement("strength", "2026-07-14T16:25:24Z", 0.11),
			measurement("decoupled", "2026-07-14T16:25:24Z", 0.11),
		]);
		measurementsStore.actions.updateFrame([
			measurement("strength", "2026-07-14T16:25:25Z", 0.22),
			measurement("decoupled", "2026-07-14T16:25:25Z", 0.22),
		]);

		const buffer = measurementsStore.state.measurements["BTC/USD"]?.leadlag;

		expect(measurementTickCount(buffer)).toBe(2);
		expect(latestMeasurementReadings(buffer)[0]?.raw).toBe(0.22);
		expect(headlineSeriesFromBuffer(buffer, "strength")).toEqual([0.11, 0.22]);
	});

	it("preserves distinct observation epochs carried by one Thesis frame", () => {
		measurementsStore.actions.updateFrame([
			measurement("strength", "2026-07-14T16:25:24Z", 0.11),
			measurement("strength", "2026-07-14T16:25:25Z", 0.22),
		]);

		const buffer = measurementsStore.state.measurements["BTC/USD"]?.leadlag;

		expect(measurementTickCount(buffer)).toBe(2);
		expect(headlineSeriesFromBuffer(buffer, "strength")).toEqual([0.11, 0.22]);
	});

	it("mutates buffers in place without cloning the full symbol map", () => {
		const symbols = Array.from({ length: 64 }, (_, index) => `SYM${index}/USD`);

		for (const symbol of symbols) {
			measurementsStore.actions.updateFrame([
				{
					...measurement("strength", "2026-07-14T16:25:24Z", 0.11),
					symbol,
				},
			]);
		}

		const mapBefore = measurementsStore.state.measurements;

		measurementsStore.actions.updateFrame([
			{
				...measurement("strength", "2026-07-14T16:25:25Z", 0.44),
				symbol: "SYM0/USD",
			},
		]);

		expect(measurementsStore.state.measurements).toBe(mapBefore);
		expect(measurementTickCount(mapBefore["SYM0/USD"]?.leadlag)).toBe(2);
	});

	it("routes thesis ticks through applyFramePayload", () => {
		applyFramePayload({
			measurements: [measurement("strength", "2026-07-14T16:25:24Z", 0.11)],
			tick: { count: 12 },
		});

		expect(
			measurementTickCount(
				measurementsStore.state.measurements["BTC/USD"]?.leadlag,
			),
		).toBe(1);
		expect(tickStore.state.frame?.count).toBe(12);
		expect(
			latestPublishedStamp(
				measurementsStore.state.measurements["BTC/USD"]?.leadlag,
			),
		).toBeTruthy();
		expect(measurementGroupKey("BTC/USD", "leadlag")).toBe(
			"BTC/USD\u0000leadlag",
		);
	});

	it("updates tick on every payload", () => {
		applyFramePayload({
			measurements: [measurement("strength", "2026-07-14T16:25:24Z", 0.11)],
			tick: { count: 99 },
		});

		expect(tickStore.state.frame?.count).toBe(99);

		applyFramePayload({
			measurements: [measurement("strength", "2026-07-14T16:25:25Z", 0.22)],
			tick: { count: 100 },
		});

		expect(tickStore.state.frame?.count).toBe(100);
	});
});
