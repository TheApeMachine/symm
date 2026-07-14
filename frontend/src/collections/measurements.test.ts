import { beforeEach, describe, expect, it } from "vitest";
import type { Measurement } from "#/types/measurement";
import {
	flattenMeasurementBuffer,
	headlineSeriesFromBuffer,
	latestMeasurementReadings,
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
		}));
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
});
