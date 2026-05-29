import { beforeEach, describe, expect, it } from "vitest";

import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";

const firstAt = "2026-05-27T23:40:00.000Z";
const secondAt = "2026-05-27T23:40:10.000Z";
const firstSec = Date.parse(firstAt) / 1000;
const secondSec = Date.parse(secondAt) / 1000;

describe("PredictionsDataProvider", () => {
	beforeEach(() => {
		PredictionsDataProvider.reset();
	});

	it("plots scaled engine pulse averages and the projected dashed prediction line", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push(reading);
		});

		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 1,
			phase: "scan",
			measurements: 7,
			open: 2,
			ts: firstAt,
			avg_prediction: 0.005,
			avg_prediction_multiple: 0.5,
			avg_error: 0.002,
			avg_error_multiple: 0.2,
		});

		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 2,
			phase: "scan",
			measurements: 9,
			open: 2,
			ts: secondAt,
			avg_prediction: 0.006,
			avg_prediction_multiple: 0.6,
			avg_error: 0.003,
			avg_error_multiple: 0.3,
		});

		unregister();

		expect(readings).toEqual([
			{ kind: "average", x: firstSec, value: 0.5 },
			{ kind: "error", x: firstSec, value: 0.2 },
			{ kind: "average", x: secondSec, value: 0.6 },
			{
				kind: "prediction",
				x: secondSec + (secondSec - firstSec),
				value: 0.6,
			},
			{ kind: "error", x: secondSec, value: 0.3 },
		]);
	});

	it("falls back to raw return fields for older engine pulse frames", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push(reading);
		});

		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 1,
			phase: "scan",
			measurements: 7,
			open: 2,
			ts: firstAt,
			avg_prediction: 0.005,
			avg_error: 0.002,
		});

		unregister();

		expect(readings).toEqual([
			{ kind: "average", x: firstSec, value: 0.005 },
			{ kind: "error", x: firstSec, value: 0.002 },
		]);
	});

	it("ignores per-symbol prediction events on the aggregate chart", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push(reading);
		});

		PredictionsDataProvider.ingest({
			event: "prediction",
			source: "perspective:microstructure",
			symbol: "BTC/EUR",
			value: 0.02,
			ts: firstAt,
			due_at: secondAt,
			runway_ms: 10000,
		});

		unregister();

		expect(readings).toEqual([]);
	});

	it("ignores per-symbol settled prediction events on the aggregate chart", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push(reading);
		});

		PredictionsDataProvider.ingest({
			event: "prediction_settled",
			ts: secondAt,
			predicted_at: firstAt,
			due_at: secondAt,
			symbol: "BTC/EUR",
			source: "perspective:microstructure",
			predicted_return: 0.02,
			actual_return: 0.018,
			error: 0.002,
		});

		unregister();

		expect(readings).toEqual([]);
	});

	it("keeps the latest engine pulse snapshot", () => {
		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 9,
			phase: "scan",
			measurements: 11,
			open: 3,
			ts: firstAt,
			avg_prediction: 0.006,
			avg_prediction_multiple: 0.6,
		});

		expect(PredictionsDataProvider.snapshot()).toMatchObject({
			event: "engine_pulse",
			seq: 9,
			avg_prediction: 0.006,
			avg_prediction_multiple: 0.6,
		});
	});
});
