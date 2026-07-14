import { bench, describe } from "vitest";
import { Circular } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/measurements";
import { hawkesCurveFromBuffer } from "./hawkes-curve";

const fitFrom = "2026-07-14T17:00:00.000Z";
const fitThrough = "2026-07-14T17:00:10.000Z";
const reading = (
	at: string,
	metric: string,
	side: string,
	raw: number,
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

const buffer = Circular<MeasurementEpoch>(50);

for (let index = 0; index < 50; index += 1) {
	const at = new Date(Date.parse(fitThrough) + (index + 1) * 250).toISOString();
	const readings = [
		reading(at, "conditional_intensity", "buy", 1.5),
		reading(at, "conditional_intensity", "sell", 1.25),
	];

	if (index === 0) {
		readings.push(
			reading(at, "baseline_intensity", "buy", 0.5),
			reading(at, "baseline_intensity", "sell", 0.5),
			reading(at, "decay_rate", "", 1),
		);
	}

	buffer.push({ at, publishedAt: at, readings });
}

describe("hawkes curve", () => {
	bench("reconstructs the retained fitted intensity path", () => {
		hawkesCurveFromBuffer(buffer);
	});
});
