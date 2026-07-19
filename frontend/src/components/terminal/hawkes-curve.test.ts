import { beforeEach, describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/types";
import {
	hawkesCurveFromBuffer,
	hawkesIntensityAt,
	hawkesSeriesFromBuffer,
	latestHawkesRaw,
	resetHawkesFitRetention,
	retainHawkesModelEpoch,
} from "./hawkes-curve";

const measurement = (
	at: string,
	metric: string,
	side: string,
	raw: number,
	fitFrom: string,
	fitThrough: string,
): Measurement => ({
	source: "hawkes",
	metric,
	subject: "hawkes_process",
	stream: "hawkes",
	symbol: "BTC/USD",
	side,
	at,
	raw,
	normalized: null,
	uncertainty: null,
	validity: { state: "provisional", readiness: "model" },
	scale: { kind: "observation_window", from: fitFrom, through: fitThrough },
});

const epoch = (at: string, readings: Measurement[]): MeasurementEpoch => ({
	at,
	publishedAt: at,
	readings,
});

describe("hawkes curve", () => {
	beforeEach(() => {
		resetHawkesFitRetention();
	});

	it("reconstructs an excitation jump followed by exponential decay", () => {
		const buffer = Circular<MeasurementEpoch>(8);
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const firstAt = "2026-07-14T17:00:11.000Z";
		const nextAt = "2026-07-14T17:00:12.000Z";
		const nextIntensity = 1 + 2 * Math.exp(-1);

		buffer.push(
			epoch(firstAt, [
				measurement(
					firstAt,
					"baseline_intensity",
					"buy",
					0.6,
					fitFrom,
					fitThrough,
				),
				measurement(
					firstAt,
					"baseline_intensity",
					"sell",
					0.4,
					fitFrom,
					fitThrough,
				),
				measurement(firstAt, "decay_rate", "", 1, fitFrom, fitThrough),
				measurement(firstAt, "spectral_radius", "", 0.72, fitFrom, fitThrough),
				measurement(
					firstAt,
					"conditional_intensity",
					"buy",
					0.9,
					fitFrom,
					fitThrough,
				),
				measurement(
					firstAt,
					"conditional_intensity",
					"sell",
					0.6,
					fitFrom,
					fitThrough,
				),
			]),
		);
		buffer.push(
			epoch(nextAt, [
				measurement(
					nextAt,
					"conditional_intensity",
					"buy",
					nextIntensity * 0.6,
					fitFrom,
					fitThrough,
				),
				measurement(
					nextAt,
					"conditional_intensity",
					"sell",
					nextIntensity * 0.4,
					fitFrom,
					fitThrough,
				),
			]),
		);

		const segments = hawkesCurveFromBuffer(buffer);
		const segment = segments[0];

		expect(segments).toHaveLength(1);

		if (segment === undefined) {
			throw new Error("expected one identifiable Hawkes curve segment");
		}

		expect(segment.beforeArrival).toBeCloseTo(1.5);
		expect(segment.afterArrival).toBeCloseTo(3);
		expect(hawkesIntensityAt(segment, Date.parse(firstAt) + 500)).toBeCloseTo(
			1 + 2 * Math.exp(-0.5),
		);
		expect(latestHawkesRaw(buffer, "spectral_radius")).toBe(0.72);
	});

	it("does not fabricate a curve without an identifiable fitted epoch", () => {
		const buffer = Circular<MeasurementEpoch>(2);
		const at = "2026-07-14T17:00:11.000Z";

		buffer.push(
			epoch(at, [
				measurement(at, "arrival_rate", "buy", 2, at, at),
				measurement(at, "arrival_rate", "sell", 1, at, at),
			]),
		);

		expect(hawkesCurveFromBuffer(buffer)).toEqual([]);
	});

	it("does not carry parameters across different fit epochs", () => {
		const buffer = Circular<MeasurementEpoch>(2);
		const priorAt = "2026-07-14T17:00:11.000Z";
		const currentAt = "2026-07-14T17:00:12.000Z";

		buffer.push(
			epoch(priorAt, [
				measurement(priorAt, "spectral_radius", "", 0.72, priorAt, priorAt),
			]),
		);
		buffer.push(
			epoch(currentAt, [
				measurement(
					currentAt,
					"conditional_intensity",
					"buy",
					2,
					currentAt,
					currentAt,
				),
				measurement(
					currentAt,
					"conditional_intensity",
					"sell",
					1,
					currentAt,
					currentAt,
				),
			]),
		);

		expect(latestHawkesRaw(buffer, "spectral_radius")).toBeNull();
	});

	it("joins evaluation intensities to the retained fit epoch", () => {
		const buffer = Circular<MeasurementEpoch>(4);
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const firstAt = "2026-07-14T17:00:11.000Z";
		const nextAt = "2026-07-14T17:00:12.000Z";
		const nextIntensity = 1 + 2 * Math.exp(-1);

		buffer.push(
			epoch(fitThrough, [
				measurement(
					fitThrough,
					"baseline_intensity",
					"buy",
					0.6,
					fitFrom,
					fitThrough,
				),
				measurement(
					fitThrough,
					"baseline_intensity",
					"sell",
					0.4,
					fitFrom,
					fitThrough,
				),
				measurement(fitThrough, "decay_rate", "", 1, fitFrom, fitThrough),
				measurement(
					fitThrough,
					"spectral_radius",
					"",
					0.72,
					fitFrom,
					fitThrough,
				),
			]),
		);
		buffer.push(
			epoch(firstAt, [
				measurement(
					firstAt,
					"conditional_intensity",
					"buy",
					0.9,
					fitFrom,
					firstAt,
				),
				measurement(
					firstAt,
					"conditional_intensity",
					"sell",
					0.6,
					fitFrom,
					firstAt,
				),
			]),
		);
		buffer.push(
			epoch(nextAt, [
				measurement(
					nextAt,
					"conditional_intensity",
					"buy",
					nextIntensity * 0.6,
					fitFrom,
					nextAt,
				),
				measurement(
					nextAt,
					"conditional_intensity",
					"sell",
					nextIntensity * 0.4,
					fitFrom,
					nextAt,
				),
			]),
		);

		const segments = hawkesCurveFromBuffer(buffer);

		expect(segments).toHaveLength(1);
		expect(segments[0]?.afterArrival).toBeCloseTo(3);
		expect(latestHawkesRaw(buffer, "spectral_radius")).toBe(0.72);
	});

	it("samples exponential decay after an impulse across the display window", () => {
		const buffer = Circular<MeasurementEpoch>(4);
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const eventAt = "2026-07-14T17:00:11.000Z";
		const now = Date.parse(eventAt) + 4_000;

		buffer.push(
			epoch(fitThrough, [
				measurement(
					fitThrough,
					"baseline_intensity",
					"buy",
					0.5,
					fitFrom,
					fitThrough,
				),
				measurement(
					fitThrough,
					"baseline_intensity",
					"sell",
					0.5,
					fitFrom,
					fitThrough,
				),
				measurement(fitThrough, "decay_rate", "", 1, fitFrom, fitThrough),
			]),
		);
		buffer.push(
			epoch(eventAt, [
				measurement(
					eventAt,
					"conditional_intensity",
					"buy",
					1.8,
					fitFrom,
					eventAt,
				),
				measurement(
					eventAt,
					"conditional_intensity",
					"sell",
					1.2,
					fitFrom,
					eventAt,
				),
			]),
		);

		const series = hawkesSeriesFromBuffer(buffer, now, 64);

		expect(series).not.toBeNull();

		if (series === null) {
			throw new Error("expected dense Hawkes series");
		}

		const peak = Math.max(...series.samples);
		const latest = series.samples.at(-1) ?? 0;

		expect(peak).toBeGreaterThan(2.5);
		expect(latest).toBeLessThan(peak);
		expect(latest).toBeGreaterThan(series.baseline);
		expect(series.samples.length).toBe(64);
	});

	it("keeps the fitted curve after the model epoch leaves the buffer", () => {
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const firstAt = "2026-07-14T17:00:11.000Z";
		const nextAt = "2026-07-14T17:00:12.000Z";
		const nextIntensity = 1 + 2 * Math.exp(-1);

		retainHawkesModelEpoch(
			epoch(fitThrough, [
				measurement(
					fitThrough,
					"baseline_intensity",
					"buy",
					0.6,
					fitFrom,
					fitThrough,
				),
				measurement(
					fitThrough,
					"baseline_intensity",
					"sell",
					0.4,
					fitFrom,
					fitThrough,
				),
				measurement(fitThrough, "decay_rate", "", 1, fitFrom, fitThrough),
				measurement(
					fitThrough,
					"spectral_radius",
					"",
					0.72,
					fitFrom,
					fitThrough,
				),
			]),
		);

		const buffer = Circular<MeasurementEpoch>(2);

		buffer.push(
			epoch(firstAt, [
				measurement(
					firstAt,
					"conditional_intensity",
					"buy",
					0.9,
					fitFrom,
					firstAt,
				),
				measurement(
					firstAt,
					"conditional_intensity",
					"sell",
					0.6,
					fitFrom,
					firstAt,
				),
			]),
		);
		buffer.push(
			epoch(nextAt, [
				measurement(
					nextAt,
					"conditional_intensity",
					"buy",
					nextIntensity * 0.6,
					fitFrom,
					nextAt,
				),
				measurement(
					nextAt,
					"conditional_intensity",
					"sell",
					nextIntensity * 0.4,
					fitFrom,
					nextAt,
				),
			]),
		);

		const segments = hawkesCurveFromBuffer(buffer);

		expect(segments).toHaveLength(1);
		expect(segments[0]?.afterArrival).toBeCloseTo(3);
		expect(latestHawkesRaw(buffer, "spectral_radius")).toBe(0.72);
	});

	it("builds a series from compact metrics maps retained in the buffer", () => {
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const firstAt = "2026-07-14T17:00:11.000Z";
		const nextAt = "2026-07-14T17:00:12.000Z";
		const scale = {
			kind: "observation_window",
			from: fitFrom,
			through: fitThrough,
		};
		const compact = (
			at: string,
			metrics: Record<string, number>,
		): Measurement => ({
			source: "hawkes",
			symbol: "BTC/USD",
			stream: "hawkes",
			at,
			raw: 0,
			normalized: null,
			uncertainty: null,
			validity: { state: "provisional", readiness: "model" },
			scale,
			metrics,
		});

		const epochs = [
			epoch(firstAt, [
				compact(firstAt, {
					"baseline_intensity:buy": 0.6,
					"baseline_intensity:sell": 0.4,
					decay_rate: 1,
					spectral_radius: 0.72,
					"conditional_intensity:buy": 0.9,
					"conditional_intensity:sell": 0.6,
				}),
			]),
			epoch(nextAt, [
				compact(nextAt, {
					"conditional_intensity:buy": 1.2,
					"conditional_intensity:sell": 0.8,
				}),
			]),
		];

		const series = hawkesSeriesFromBuffer(epochs, Date.parse(nextAt) + 1000);

		expect(series).not.toBeNull();
		expect(series?.samples.length).toBeGreaterThan(1);
		expect(series?.peakExcess).toBeCloseTo(1.0);
		expect(series?.fit).toBe(fitFrom);
		expect(latestHawkesRaw(epochs, "spectral_radius")).toBe(0.72);
	});
});
