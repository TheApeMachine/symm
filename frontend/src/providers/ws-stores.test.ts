import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
	retainManifoldBinary: vi.fn(() => true),
	repaintTerminalFluidChart: vi.fn(),
}));

vi.mock("#/components/charts/fluid", () => ({
	repaintTerminalFluidChart: mocks.repaintTerminalFluidChart,
}));

vi.mock("#/collections/app", () => ({
	appStore: { state: { focusSymbol: "BTC/USD" } },
}));

vi.mock("#/providers/manifold-binary", () => ({
	retainManifoldBinary: mocks.retainManifoldBinary,
}));

import {
	attach,
	paintRegistered,
	registerPainter,
} from "#/providers/ws-stores";

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

describe("ws-stores", () => {
	let animationFrame: FrameRequestCallback | null;

	beforeEach(() => {
		animationFrame = null;
		mocks.retainManifoldBinary.mockClear();
		mocks.repaintTerminalFluidChart.mockClear();
		vi.stubGlobal(
			"requestAnimationFrame",
			vi.fn((callback: FrameRequestCallback) => {
				animationFrame = callback;
				return 1;
			}),
		);
	});

	it("registers painters under backend wire keys and dispatches updates", () => {
		const paint = vi.fn();
		const unregister = registerPainter("measurements", paint);

		paintRegistered("measurements", { source: "hawkes" });

		expect(paint).toHaveBeenCalledTimes(1);
		expect(paint).toHaveBeenCalledWith({ source: "hawkes" });

		unregister();
		paintRegistered("measurements", { source: "sentiment" });

		expect(paint).toHaveBeenCalledTimes(1);
	});

	it("coalesces DRAW messages and paints the latest value for each key", () => {
		const worker = new MockWorker();
		const measurementsPaint = vi.fn();
		const healthPaint = vi.fn();
		const unregisterMeasurements = registerPainter(
			"measurements",
			measurementsPaint,
		);
		const unregisterHealth = registerPainter("health", healthPaint);

		attach(worker as unknown as Worker);
		worker.emit({
			type: "DRAW",
			frame: {
				measurements: { epoch: 1 },
				health: { online: true },
			},
		});
		worker.emit({
			type: "DRAW",
			frame: { measurements: { epoch: 2 } },
		});

		expect(requestAnimationFrame).toHaveBeenCalledTimes(1);
		expect(measurementsPaint).not.toHaveBeenCalled();
		expect(healthPaint).not.toHaveBeenCalled();

		animationFrame?.(0);

		expect(measurementsPaint).toHaveBeenCalledOnce();
		expect(measurementsPaint).toHaveBeenCalledWith({ epoch: 2 });
		expect(healthPaint).toHaveBeenCalledOnce();
		expect(healthPaint).toHaveBeenCalledWith({ online: true });

		unregisterMeasurements();
		unregisterHealth();
	});

	it("dispatches resonance batches under the backend resonance key", () => {
		const worker = new MockWorker();
		const resonancePaint = vi.fn();
		const unregisterResonance = registerPainter("resonance", resonancePaint);
		const row = {
			symbol: "BTC/USD",
			confidence: 0.75,
			latent: [0.1, -0.2],
			forwardCurve: [0.01],
		};

		attach(worker as unknown as Worker);
		worker.emit({
			type: "DRAW",
			frame: { resonance: [row] },
		});

		animationFrame?.(0);

		expect(resonancePaint).toHaveBeenCalledOnce();
		expect(resonancePaint).toHaveBeenCalledWith([row]);

		unregisterResonance();
	});

	it("retains independently published open positions by identity", () => {
		const worker = new MockWorker();
		const positionsPaint = vi.fn();
		const unregisterPositions = registerPainter("positions", positionsPaint);
		const position = (id: string, symbol: string, status = "open") => ({
			id,
			status,
			holding: { symbol },
		});

		attach(worker as unknown as Worker);
		worker.emit({
			type: "DRAW",
			frame: { positions: [position("btc", "BTC/USD")] },
		});
		worker.emit({
			type: "DRAW",
			frame: { positions: [position("eth", "ETH/USD")] },
		});
		worker.emit({
			type: "DRAW",
			frame: { positions: [position("sol", "SOL/USD")] },
		});
		worker.emit({
			type: "DRAW",
			frame: { positions: [position("xrp", "XRP/USD")] },
		});

		animationFrame?.(0);

		expect(positionsPaint).toHaveBeenCalledOnce();
		expect(positionsPaint).toHaveBeenCalledWith([
			position("btc", "BTC/USD"),
			position("eth", "ETH/USD"),
			position("sol", "SOL/USD"),
			position("xrp", "XRP/USD"),
		]);

		worker.emit({
			type: "DRAW",
			frame: { positions: [position("eth", "ETH/USD", "closed")] },
		});
		animationFrame?.(0);

		expect(positionsPaint).toHaveBeenLastCalledWith([
			position("btc", "BTC/USD"),
			position("sol", "SOL/USD"),
			position("xrp", "XRP/USD"),
		]);

		unregisterPositions();
	});

	it("retains only the latest binary and repaints once per display frame", () => {
		const worker = new MockWorker();
		const first = new ArrayBuffer(1);
		const latest = new ArrayBuffer(2);

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW_BIN", buffer: first });
		worker.emit({ type: "DRAW_BIN", buffer: latest });

		expect(requestAnimationFrame).toHaveBeenCalledTimes(1);
		expect(mocks.retainManifoldBinary).not.toHaveBeenCalled();
		expect(mocks.repaintTerminalFluidChart).not.toHaveBeenCalled();

		animationFrame?.(0);

		expect(mocks.retainManifoldBinary).toHaveBeenCalledOnce();
		expect(mocks.retainManifoldBinary).toHaveBeenCalledWith(latest);
		expect(mocks.repaintTerminalFluidChart).toHaveBeenCalledOnce();
		expect(mocks.repaintTerminalFluidChart).toHaveBeenCalledWith("BTC/USD");
	});
});
