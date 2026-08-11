import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
	paintTerminalFluidChart: vi.fn(),
	paintTerminalResonanceChart: vi.fn(),
}));

vi.mock("#/components/charts/fluid", () => ({
	paintTerminalFluidChart: mocks.paintTerminalFluidChart,
}));

vi.mock("#/components/charts/resonance", () => ({
	paintTerminalResonanceChart: mocks.paintTerminalResonanceChart,
}));

vi.mock("#/collections/app", () => ({
	appStore: {
		state: { focusSymbol: "BTC/USD" },
		actions: { observeSymbols: () => {}, observeSources: () => {} },
	},
}));

import {
	attach,
	getLastFrame,
	paintRegistered,
	registerPainter,
	RESONANCE_FOCUS,
} from "#/providers/ws-stores";

type WorkerListener = (event: MessageEvent) => void;

class MockWorker {
	listener: WorkerListener | null = null;
	messages: unknown[] = [];

	addEventListener(_type: string, listener: WorkerListener) {
		this.listener = listener;
	}

	postMessage(message: unknown) {
		this.messages.push(message);
	}

	emit(data: unknown) {
		this.listener?.({ data } as MessageEvent);
	}
}

describe("ws-stores", () => {
	let animationFrame: FrameRequestCallback | null;

	beforeEach(() => {
		animationFrame = null;
		mocks.paintTerminalFluidChart.mockClear();
		mocks.paintTerminalResonanceChart.mockClear();
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

	it("retains sparse measurements without repainting absent identities", () => {
		const paint = vi.fn();
		const unregister = registerPainter("measurements", paint);
		const cvd = { source: "cvd", symbol: "BTC/USD", strength: 0.25 };
		const correlation = {
			source: "correlation",
			symbol: "BTC/USD",
			strength: 0.5,
		};

		paintRegistered("measurements", [cvd]);
		paintRegistered("measurements", [correlation]);

		expect(paint).toHaveBeenNthCalledWith(1, [cvd]);
		expect(paint).toHaveBeenNthCalledWith(2, [correlation]);
		expect(getLastFrame("measurements")).toEqual([cvd, correlation]);

		unregister();
	});

	it("paints every sparse measurement frame before coalescing display state", () => {
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
		expect(measurementsPaint).toHaveBeenCalledTimes(2);
		expect(measurementsPaint).toHaveBeenNthCalledWith(1, { epoch: 1 });
		expect(measurementsPaint).toHaveBeenNthCalledWith(2, { epoch: 2 });
		expect(healthPaint).not.toHaveBeenCalled();

		animationFrame?.(0);

		expect(healthPaint).toHaveBeenCalledOnce();
		expect(healthPaint).toHaveBeenCalledWith({ online: true });
		expect(worker.messages).toEqual([]);

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
		expect(mocks.paintTerminalResonanceChart).toHaveBeenCalledOnce();
		expect(mocks.paintTerminalResonanceChart).toHaveBeenCalledWith(
			[row],
			"BTC/USD",
		);

		unregisterResonance();
	});

	it("retains the direct resonance row emitted by the solver", () => {
		const worker = new MockWorker();
		const resonancePaint = vi.fn();
		const unregisterResonance = registerPainter("resonance", resonancePaint);
		const row = {
			stage: "resonance",
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-08-09T16:00:00Z",
			latent: [0.3, -0.4],
			layers: [{ state: [0.1], prediction: [0.2] }],
			surprise: 0.1,
			energy: 0.2,
			alpha: 0.05,
		};

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW", frame: { resonance: row } });
		animationFrame?.(0);

		expect(resonancePaint).toHaveBeenCalledWith([row]);

		unregisterResonance();
	});

	/*
		Every settled carrier is published so the latent cross-section has a cloud
		to plot, and the focused carrier is derived onto its own key so a chart
		cannot read a neighbour's vectors under the focused symbol's name.
	*/
	it("derives the focused carrier onto its own key", () => {
		const worker = new MockWorker();
		const batchPaint = vi.fn();
		const focusPaint = vi.fn();
		const unregisterBatch = registerPainter("resonance", batchPaint);
		const unregisterFocus = registerPainter(RESONANCE_FOCUS, focusPaint);
		const other = { symbol: "AVAX/USD", embedding: [0.07, 0.08] };
		const focused = { symbol: "BTC/USD", embedding: [0.01, -0.02] };

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW", frame: { resonance: [other, focused] } });

		animationFrame?.(0);

		expect(batchPaint).toHaveBeenCalledWith([other, focused]);
		expect(focusPaint).toHaveBeenCalledWith(focused);

		unregisterBatch();
		unregisterFocus();
	});

	it("paints no focused carrier until the focused symbol settles", () => {
		const worker = new MockWorker();
		const focusPaint = vi.fn();
		const unregisterFocus = registerPainter(RESONANCE_FOCUS, focusPaint);
		const rows = [
			{ symbol: "AVAX/USD", embedding: [0.07, 0.08] },
			{ symbol: "SOL/USD", embedding: [0.02, 0.05] },
		];

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW", frame: { resonance: rows } });

		animationFrame?.(0);

		expect(focusPaint).not.toHaveBeenCalled();

		unregisterFocus();
	});

	it("dispatches manifold batches to the fluid chart painter", () => {
		const worker = new MockWorker();
		const row = { source: "manifold", symbol: "BTC/USD" };

		attach(worker as unknown as Worker);
		worker.emit({
			type: "DRAW",
			frame: { manifold: [row] },
		});

		animationFrame?.(0);

		expect(mocks.paintTerminalFluidChart).toHaveBeenCalledOnce();
		expect(mocks.paintTerminalFluidChart).toHaveBeenCalledWith(
			[row],
			"BTC/USD",
		);
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

	it("merges independently published cognition symbols into one map", () => {
		const worker = new MockWorker();
		const cognitionPaint = vi.fn();
		const unregisterCognition = registerPainter("cognition", cognitionPaint);

		attach(worker as unknown as Worker);
		worker.emit({
			type: "DRAW",
			frame: { cognition: { "BTC/USD": { winnerRegime: "coil" } } },
		});
		worker.emit({
			type: "DRAW",
			frame: { cognition: { "ETH/USD": { winnerRegime: "lift" } } },
		});

		animationFrame?.(0);

		expect(cognitionPaint).toHaveBeenLastCalledWith({
			"BTC/USD": { winnerRegime: "coil" },
			"ETH/USD": { winnerRegime: "lift" },
		});

		worker.emit({
			type: "DRAW",
			frame: { cognition: { "BTC/USD": { winnerRegime: "flush" } } },
		});
		animationFrame?.(0);

		expect(cognitionPaint).toHaveBeenLastCalledWith({
			"BTC/USD": { winnerRegime: "flush" },
			"ETH/USD": { winnerRegime: "lift" },
		});

		unregisterCognition();
	});

	it("retains resonance carriers by symbol across sparse batches", () => {
		const worker = new MockWorker();
		const resonancePaint = vi.fn();
		const unregisterResonance = registerPainter("resonance", resonancePaint);
		const btc = { symbol: "BTC/USD", confidence: 0.5 };
		const eth = { symbol: "ETH/USD", confidence: 0.25 };

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW", frame: { resonance: [btc] } });
		worker.emit({ type: "DRAW", frame: { resonance: [eth] } });

		animationFrame?.(0);

		expect(resonancePaint).toHaveBeenLastCalledWith([btc, eth]);

		unregisterResonance();
	});

	it("retains readiness independently for every symbol", () => {
		const worker = new MockWorker();
		const readinessPaint = vi.fn();
		const unregisterReadiness = registerPainter("readiness", readinessPaint);
		const btc = { symbol: "BTC/USD", hawkes: true };
		const eth = { symbol: "ETH/USD", correlation: true };

		attach(worker as unknown as Worker);
		worker.emit({ type: "DRAW", frame: { readiness: [btc] } });
		worker.emit({ type: "DRAW", frame: { readiness: [eth] } });

		animationFrame?.(0);

		expect(readinessPaint).toHaveBeenLastCalledWith([btc, eth]);

		unregisterReadiness();
	});
});
