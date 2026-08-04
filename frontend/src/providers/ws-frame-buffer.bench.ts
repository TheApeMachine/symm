import { bench, describe } from "vitest";
import { WorkerFrameBuffer } from "#/providers/ws-frame-buffer";

const resonanceUpdates = Array.from({ length: 256 }, (_, index) => ({
	resonance: [
		{
			symbol: `SYMBOL-${index % 64}`,
			latent: Array.from({ length: 51 }, (__, dimension) => dimension + index),
			forwardCurve: Array.from(
				{ length: (index % 20) + 1 },
				(___, horizon) => horizon,
			),
		},
	],
}));

describe("WorkerFrameBuffer", () => {
	bench("coalesces sparse resonance updates by identity", () => {
		const buffer = new WorkerFrameBuffer();

		for (const update of resonanceUpdates) {
			buffer.merge(update);
		}

		buffer.take();
	});
});
