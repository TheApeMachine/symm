import { bench, describe } from "vitest";
import { Circular } from "#/collections/circular";
import type { ResonanceFrame } from "#/collections/types";
import { updatePredictionHistory } from "#/components/charts/prediction";
import { paintKernelList } from "#/components/terminal/kernel-list";
import type { Measurement } from "#/types/measurement";

const predictionHistory = Circular<ResonanceFrame>(1024);
let predictionSample = 0;

const row = (raw: number): Measurement => ({
	source: "pumpdump",
	metric: "raw",
	symbol: "BTC/USD",
	at: new Date().toISOString(),
	raw,
	normalized: raw,
	uncertainty: null,
	validity: { state: "valid", readiness: "ready" },
	scale: { kind: "unit", from: "0", through: "1" },
});

describe("DRAW paint", () => {
	bench("paints a measurements delta", () => {
		paintKernelList([row(Math.random())], "BTC/USD");
	});

	bench("appends a predictive-coding delta to bounded history", () => {
		predictionSample += 1;
		updatePredictionHistory(predictionHistory, [
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: String(predictionSample),
				layers: [{ state: [0.1], prediction: [0.2] }],
			},
		]);
	});
});
