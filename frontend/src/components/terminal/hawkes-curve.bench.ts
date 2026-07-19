import { bench, describe } from "vitest";
import { Circular } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/types";
import { hawkesCurveFromBuffer, hawkesSeriesFromBuffer } from "./hawkes-curve";

const fitFrom = "2026-07-14T17:00:00.000Z";
const fitThrough = "2026-07-14T17:00:10.000Z";
const reading = (
	at: string,
	metric: string,
	side: string,
	raw: number,
	through = at,
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
	scale: { kind: "observation_window", from: fitFrom, through },
});

const buffer = Circular<MeasurementEpoch>(50);

buffer.push({
	at: fitThrough,
	publishedAt: fitThrough,
	readings: [
		reading(fitThrough, "baseline_intensity", "buy", 0.5, fitThrough),
		reading(fitThrough, "baseline_intensity", "sell", 0.5, fitThrough),
		reading(fitThrough, "decay_rate", "", 1, fitThrough),
	],
});

for (let index = 0; index < 49; index += 1) {
	const at = new Date(Date.parse(fitThrough) + (index + 1) * 250).toISOString();

	buffer.push({
		at,
		publishedAt: at,
		readings: [
			reading(at, "conditional_intensity", "buy", 1.5),
			reading(at, "conditional_intensity", "sell", 1.25),
		],
	});
}

describe("hawkes curve", () => {
	bench("reconstructs the retained fitted intensity path", () => {
		hawkesCurveFromBuffer(buffer);
	});

	bench("samples the dense impulse-and-decay series", () => {
		hawkesSeriesFromBuffer(buffer, Date.parse(fitThrough) + 12_500);
	});
});
