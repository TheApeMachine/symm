import { bench, describe } from "vitest";
import type { ResonanceFrame } from "#/collections/types";
import { predictiveCodingSeries } from "#/components/charts/prediction/series";

const frames: ResonanceFrame[] = Array.from({ length: 1024 }, (_, index) => ({
	source: "resonance",
	symbol: "BTC/USD",
	at: String(index),
	layers: Array.from({ length: 3 }, (__, layerIndex) => ({
		state: Array.from({ length: 5 }, (___, dimension) =>
			Math.sin(index + layerIndex + dimension),
		),
		prediction: Array.from({ length: 5 }, (___, dimension) =>
			Math.cos(index + layerIndex + dimension),
		),
	})),
	expectedReturn: Math.sin(index) / 100,
	uncertainty: Math.abs(Math.cos(index)) / 100,
	incrementalMSE: 0.001,
	incrementalSkillLowerBound: 0.0001,
	calibrationSamples: index,
	returnReady: true,
}));

describe("predictive-coding chart", () => {
	bench("derives hierarchy and return-head traces", () => {
		predictiveCodingSeries(frames);
	});
});
