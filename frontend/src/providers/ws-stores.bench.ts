import { bench, describe } from "vitest";
import { attach, measurementsStore } from "#/providers/ws-stores";

const measurementFrames = Array.from({ length: 64 }, (_, index) => ({
	measurements: [
		{
			source: `kernel-${index}`,
			symbol: "BTC/USD",
			metrics: { strength: { raw: index / 64, normalized: index / 64 } },
		},
	],
}));

class MockWorker {
	listener: ((event: MessageEvent) => void) | null = null;

	addEventListener(_type: string, listener: (event: MessageEvent) => void) {
		this.listener = listener;
	}

	postMessage() {}
}

describe("ws-stores", () => {
	const worker = new MockWorker();
	attach(worker as unknown as Worker);

	bench("ingests sparse measurement frames through the sink registry", () => {
		for (const frame of measurementFrames) {
			worker.listener?.({ data: { type: "DRAW", frame } } as MessageEvent);
		}

		// force the rAF flush synchronously in bench by not awaiting it; read state
		void measurementsStore.state.measurements;
	});
});
