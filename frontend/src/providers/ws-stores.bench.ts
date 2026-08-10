import { bench, describe } from "vitest";
import { paintRegistered, registerPainter } from "#/providers/ws-stores";

const measurementFrames = Array.from({ length: 64 }, (_, index) => [
	{
		source: `kernel-${index}`,
		symbol: "BTC/USD",
		metrics: {
			strength: { raw: index / 64, normalized: index / 64 },
		},
	},
]);

describe("ws-stores", () => {
	registerPainter("measurements", () => {});

	bench("paints sparse measurement frames directly", () => {
		for (const frame of measurementFrames) {
			paintRegistered("measurements", frame);
		}
	});
});
