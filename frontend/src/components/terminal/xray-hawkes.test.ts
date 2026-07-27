import { describe, expect, it } from "vitest";
import type { Measurement, MeasurementEpoch } from "#/collections/types";
import { hawkesSeriesFromBuffer } from "#/components/terminal/hawkes-curve";

const epoch = (at: string, readings: Measurement[]): MeasurementEpoch => ({
	at,
	readings,
	publishedAt: at,
});

const measurement = (
	at: string,
	metric: string,
	side: string,
	raw: number,
	fitFrom: string,
	fitThrough: string,
): Measurement => ({
	source: "hawkes",
	symbol: "BTC/USD",
	stream: "hawkes",
	at,
	metric,
	side,
	raw,
	normalized: null,
	uncertainty: null,
	validity: { state: "provisional", readiness: "model" },
	scale: {
		kind: "observation_window",
		from: fitFrom,
		through: fitThrough,
	},
});

describe("xray hawkes retained epochs", () => {
	it("builds a curve once multiple focused conditional-intensity epochs are retained", () => {
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const epochs: MeasurementEpoch[] = [
			epoch(fitThrough, [
				measurement(fitThrough, "baseline_intensity", "buy", 0.6, fitFrom, fitThrough),
				measurement(fitThrough, "baseline_intensity", "sell", 0.4, fitFrom, fitThrough),
				measurement(fitThrough, "decay_rate", "", 1, fitFrom, fitThrough),
			]),
			epoch("2026-07-14T17:00:11.000Z", [
				measurement("2026-07-14T17:00:11.000Z", "conditional_intensity", "buy", 0.9, fitFrom, fitThrough),
				measurement("2026-07-14T17:00:11.000Z", "conditional_intensity", "sell", 0.6, fitFrom, fitThrough),
			]),
			epoch("2026-07-14T17:00:12.000Z", [
				measurement("2026-07-14T17:00:12.000Z", "conditional_intensity", "buy", 1.14, fitFrom, fitThrough),
				measurement("2026-07-14T17:00:12.000Z", "conditional_intensity", "sell", 0.76, fitFrom, fitThrough),
			]),
		];

		const series = hawkesSeriesFromBuffer(epochs, Date.parse("2026-07-14T17:00:12.000Z"), 32);

		expect(series).not.toBeNull();
		expect(series?.samples.length).toBe(32);
	});
});
