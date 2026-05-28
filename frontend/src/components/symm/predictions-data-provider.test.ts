import { beforeEach, describe, expect, it } from "vitest";

import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";

const firstPulseAt = "2026-05-27T23:40:00.000Z";
const secondPulseAt = "2026-05-27T23:40:10.000Z";
const firstPulseSec = Date.parse(firstPulseAt) / 1000;
const secondPulseSec = Date.parse(secondPulseAt) / 1000;

describe("PredictionsDataProvider", () => {
	beforeEach(() => {
		PredictionsDataProvider.reset();
	});

	it("plots aggregate prediction and average error from engine pulses", () => {
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
			ts: firstPulseAt,
			avg_prediction: 0.005,
			avg_error: 0.0038,
		});

		unregister();

		expect(readings).toEqual([
			{ kind: "average", x: firstPulseSec, value: 0.005 },
			{ kind: "error", x: firstPulseSec, value: 0.0038 },
		]);
	});

	it("projects the average prediction one observed pulse interval ahead", () => {
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
			ts: firstPulseAt,
			avg_prediction: 0.005,
			avg_error: 0.0038,
		});
		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 2,
			phase: "scan",
			measurements: 9,
			open: 2,
			ts: secondPulseAt,
			avg_prediction: 0.0054,
			avg_error: 0.004,
		});

		unregister();

		expect(readings).toEqual([
			{ kind: "average", x: firstPulseSec, value: 0.005 },
			{ kind: "error", x: firstPulseSec, value: 0.0038 },
			{ kind: "average", x: secondPulseSec, value: 0.0054 },
			{ kind: "error", x: secondPulseSec, value: 0.004 },
			{ kind: "prediction", x: secondPulseSec + 10, value: 0.0054 },
		]);
	});

	it("ignores per-symbol forecasts so the chart stays aggregate", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push(reading);
		});

		PredictionsDataProvider.ingest({
			event: "prediction",
			source: "perspective:microstructure",
			symbol: "BTC/EUR",
			value: 0.02,
			ts: firstPulseAt,
			due_at: secondPulseAt,
		});
		PredictionsDataProvider.ingest({
			Source: "fluid",
			Symbol: "ETH/EUR",
			PredictedReturn: 0.04,
			ActualReturn: 0.01,
			Error: 0.03,
			PredictedAt: firstPulseAt,
			DueAt: secondPulseAt,
			SettledAt: secondPulseAt,
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
			ts: firstPulseAt,
			avg_prediction: 0.006,
			avg_error: 0.0042,
		});

		expect(PredictionsDataProvider.snapshot()).toMatchObject({
			event: "engine_pulse",
			seq: 9,
			avg_prediction: 0.006,
			avg_error: 0.0042,
		});
	});
});
