import { bench, describe } from "vitest";
import { predictiveCodingSeries } from "#/components/charts/prediction/series";
import { Value as ResonanceFrame } from "#/types/value";

/*
Frames built the way the hub builds them: layer payloads appended as JSON
under their keys, so the series helpers decode them through Value.lookup.
*/
const frames: ResonanceFrame[] = Array.from({ length: 1024 }, (_, index) => {
	const frame = new ResonanceFrame({
		source: "resonance",
		symbol: "BTC/USD",
		at: String(index),
	});

	frame.append(
		Array.from({ length: 3 }, (__, layerIndex) => ({
			state: Array.from({ length: 5 }, (___, dimension) =>
				Math.sin(index + layerIndex + dimension),
			),
			prediction: Array.from({ length: 5 }, (___, dimension) =>
				Math.cos(index + layerIndex + dimension),
			),
		})),
		"layers",
	);

	frame.append(
		{
			forwardCurve: [Math.sin(index) / 100],
			forwardRetention: [1],
			supportedHorizon: 1,
			probeHorizon: 1,
			posterior: [{ Scale: Math.abs(Math.cos(index)) / 100 }],
		},
		"forecast",
	);

	frame.append(
		[
			{ name: "task_relative_precision", raw: 0.75 },
			{ name: "task_relative_precision_ready", raw: 1 },
			{ name: "calibration_samples", raw: index },
		],
		"metrics",
	);

	return frame;
});

describe("predictive-coding chart", () => {
	bench("derives hierarchy and return-head traces", () => {
		predictiveCodingSeries(frames);
	});
});
