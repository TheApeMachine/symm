import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/measurements";
import {
	hawkesCurveFromBuffer,
	hawkesIntensityAt,
	latestHawkesRaw,
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
});
