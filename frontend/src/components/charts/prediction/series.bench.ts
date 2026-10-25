import { bench, describe } from "vitest";
import { predictiveCodingSeries } from "#/components/charts/prediction/series";
import { encodeJsonBytes } from "#/lib/json-bytes";
import { Value as ResonanceFrame } from "#/types/value";

const resonanceFrame = (index: number): ResonanceFrame => {
	const prefix = "frame/resonance/BTC-USD";
	const frame = new ResonanceFrame({
		prefix,
		scope: "BTC/USD",
		origin: "resonance",
		tick: BigInt(index),
	});

	const addMetric = (name: string, raw: number) => {
		frame.index.push(`${prefix}/${name}`);
		frame.layers.push(encodeJsonBytes([raw]));
	};

	for (let layerIndex = 0; layerIndex < 3; layerIndex++) {
		for (let dimension = 0; dimension < 5; dimension++) {
			addMetric(
				`layer_${layerIndex}_state_${dimension}`,
				Math.sin(index + layerIndex + dimension),
			);
			addMetric(
				`layer_${layerIndex}_prediction_${dimension}`,
				Math.cos(index + layerIndex + dimension),
			);
		}
	}

	addMetric("forward_curve_0", Math.sin(index) / 100);
	addMetric("task_relative_precision", 0.75);

	return frame;
};

const frames: ResonanceFrame[] = Array.from({ length: 1024 }, (_, index) =>
	resonanceFrame(index),
);

describe("predictive-coding chart", () => {
	bench("derives hierarchy and return-head traces", () => {
		predictiveCodingSeries(frames);
	});
});
