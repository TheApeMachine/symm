import { describe, expect, it } from "vitest";

import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";

const dueAt = "2026-05-27T12:00:00.000Z";
const dueSec = Date.parse(dueAt) / 1000;
const settledAt = "2026-05-27T12:00:15.000Z";
const settledSec = Date.parse(settledAt) / 1000;

describe("PredictionsDataProvider", () => {
	it("plots predictions at due time and ground truth at settlement time", () => {
		const readings: Array<{ kind: string; x: number; value: number }> = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			readings.push({
				kind: reading.kind,
				x: reading.x,
				value: reading.value,
			});
		});

		PredictionsDataProvider.ingest({
			event: "prediction",
			source: "pumpdump",
			symbol: "MNT/EUR",
			value: 0.177,
			due_at: dueAt,
		});

		PredictionsDataProvider.ingest({
			Source: "pumpdump",
			Symbol: "MNT/EUR",
			PredictedReturn: 0.177,
			ActualReturn: 0.002,
			Error: 0.175,
			DueAt: dueAt,
			SettledAt: settledAt,
		});

		unregister();

		expect(readings).toHaveLength(3);
		expect(readings[0]).toMatchObject({
			kind: "predicted",
			x: dueSec,
			value: 17.7,
		});
		expect(readings[1]).toMatchObject({
			kind: "actual",
			x: settledSec,
			value: 0.2,
		});
		expect(readings[2]).toMatchObject({
			kind: "error",
			x: settledSec,
			value: 17.5,
		});
	});

	it("does not duplicate open predictions before settlement", () => {
		const predictedX: number[] = [];

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			if (reading.kind === "predicted" && reading.source === "hawkes") {
				predictedX.push(reading.x);
			}
		});

		PredictionsDataProvider.ingest({
			event: "prediction",
			source: "hawkes",
			symbol: "MNT/EUR",
			value: 0.05,
			due_at: dueAt,
		});
		PredictionsDataProvider.ingest({
			event: "prediction",
			source: "hawkes",
			symbol: "MNT/EUR",
			value: 0.08,
			due_at: dueAt,
		});

		unregister();

		expect(predictedX).toEqual([dueSec]);
	});

	it("settles from PredictedAt when DueAt is missing", () => {
		const readings: Array<{ kind: string; x: number }> = [];
		let capturing = false;

		const unregister = PredictionsDataProvider.registerSink((reading) => {
			if (!capturing) {
				return;
			}

			readings.push({ kind: reading.kind, x: reading.x });
		});

		capturing = true;

		const predictedAt = "2026-05-27T11:58:00.000Z";

		PredictionsDataProvider.ingest({
			Source: "fluid",
			Symbol: "MNT/EUR",
			PredictedReturn: 0.04,
			ActualReturn: 0.01,
			Error: 0.03,
			PredictedAt: predictedAt,
		});

		unregister();

		expect(readings).toHaveLength(3);
		expect(
			readings.every((row) => row.x === Date.parse(predictedAt) / 1000),
		).toBe(true);
	});
});
