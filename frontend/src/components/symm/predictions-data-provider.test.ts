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

	it("plots the average prediction from each engine pulse at its own timestamp", () => {
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
		});

		unregister();

		expect(readings).toEqual([
			{ kind: "average", x: firstSec, value: 0.005 },
		]);
	});

	it("plots each per-symbol prediction at its own anchored ts (predictions live in time, not cycles)", () => {
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

		expect(readings).toEqual([
			{ kind: "prediction", x: firstSec, value: 0.02 },
		]);
	});

	it("plots realised return and error at the predicted_at anchor so cohorts align", () => {
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

		expect(readings).toEqual([
			{ kind: "actual", x: firstSec, value: 0.018 },
			{ kind: "error", x: firstSec, value: 0.002 },
		]);
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
		});

		expect(PredictionsDataProvider.snapshot()).toMatchObject({
			event: "engine_pulse",
			seq: 9,
			avg_prediction: 0.006,
		});
	});
});
