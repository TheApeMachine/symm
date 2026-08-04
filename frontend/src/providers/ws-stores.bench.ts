import { bench, describe, vi } from "vitest";

vi.mock("#/components/charts/fluid", () => ({
	paintTerminalFluidChart: vi.fn(),
	repaintTerminalFluidChart: vi.fn(),
}));

vi.mock("#/components/charts/resonance", () => ({
	paintTerminalResonanceChart: vi.fn(),
}));

vi.mock("#/collections/app", () => ({
	appStore: { state: { focusSymbol: "BTC/USD" } },
}));

vi.mock("#/providers/manifold-binary", () => ({
	retainManifoldBinary: vi.fn(() => true),
}));

import { attach } from "#/providers/ws-stores";

type WorkerListener = (event: MessageEvent) => void;

class MockWorker {
	listener: WorkerListener | null = null;

	addEventListener(_type: string, listener: WorkerListener) {
		this.listener = listener;
	}

	emit(data: unknown) {
		this.listener?.({ data } as MessageEvent);
	}
}

const resonanceUpdates = Array.from({ length: 256 }, (_, index) => ({
	type: "DRAW",
	frame: {
		resonance: [
			{
				symbol: `SYMBOL-${index}`,
				latent: Array.from(
					{ length: 51 },
					(__, dimension) => dimension + index,
				),
				forwardCurve: Array.from(
					{ length: (index % 20) + 1 },
					(___, horizon) => horizon,
				),
			},
		],
	},
}));

describe("ws-stores", () => {
	bench("coalesces retained resonance messages before painting", () => {
		const worker = new MockWorker();

		vi.stubGlobal(
			"requestAnimationFrame",
			vi.fn(() => 1),
		);
		attach(worker as unknown as Worker);

		for (const update of resonanceUpdates) {
			worker.emit(update);
		}
	});
});
