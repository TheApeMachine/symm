import { bench, describe, vi } from "vitest";
import { WsFeed } from "./websocket";
import { applyFramePayload } from "./ws-stores";

const mockedReact = vi.hoisted(() => ({
	cleanup: undefined as (() => void) | undefined,
}));

vi.mock("react", () => ({
	useEffect: (effect: () => undefined | (() => void)) => {
		mockedReact.cleanup = effect() ?? undefined;
	},
}));

type Listener = (event: Record<string, unknown>) => void;

class MockWebSocket {
	static readonly OPEN = 1;
	static instances: MockWebSocket[] = [];

	readonly url: string;
	readyState = MockWebSocket.OPEN;
	private listeners: Record<string, Listener[]> = {};

	constructor(url: string) {
		this.url = url;
		MockWebSocket.instances.push(this);
	}

	addEventListener(type: string, listener: Listener) {
		this.listeners[type] = [...(this.listeners[type] ?? []), listener];
	}

	close() {
		this.readyState = 3;
	}

	emit(type: string, event: Record<string, unknown> = {}) {
		for (const listener of this.listeners[type] ?? []) {
			listener(event);
		}
	}
}

const frame = {
	measurements: [
		{
			source: "fluid",
			symbol: "BTC/USD",
			at: "2026-01-01T00:00:00Z",
			status: "measured",
			elapsed: 1.0,
			entryBaseline: 0.5,
			exitBaseline: 0.3,
			categories: [
				{ type: "laminar", confidence: 0.8, surprisal: 0.2, strength: 0.6 },
			],
			metrics: { divergence: 0.1 },
		},
	],
	intents: [
		{
			Symbol: "BTC/USD",
			Action: "buy",
			Size: "0.05",
			Edge: 0.72,
			Velocity: 0.72,
			Confidence: 0.82,
			Thesis: {},
		},
	],
	balances: [
		{
			asset: "USD",
			balance: 200,
			available: 180,
			reserved: 20,
		},
	],
	positions: [
		{
			symbol: "BTC/USD",
			qty: 0.01,
			entry_price: 61000,
			mark: 61420,
			pnl: 4.2,
			return_pct: 0.0068852459,
		},
	],
	stops: {
		"BTC/USD": {
			symbol: "BTC/USD",
			stop_price: 61200,
			peak_return: 0.008,
			stop_return: 0.0032,
			momentum: 0.8,
			peak_momentum: 1,
			momentum_floor: 0.4,
			momentum_health: 0.6666666667,
			momentum_active: true,
			peak_touch_count: 1,
			stagnation_max_touches: 4,
			stagnation_health: 0.75,
			stagnation_pending: false,
			stagnation_active: true,
		},
	},
	executions: [
		{
			channel: "executions",
			type: "update",
			sequence: 1,
			data: [
				{
					exec_id: "E1",
					exec_type: "trade",
					order_id: "O1",
					symbol: "BTC/USD",
					side: "buy",
					last_qty: 0.01,
					last_price: 61420,
				},
			],
		},
	],
	decisions: [
		{
			action: "enter",
			symbol: "BTC/USD",
			at: "2026-01-01T00:00:00Z",
			utility: 0.42,
			alternatives: { hold: 0.1 },
			allocationClass: "core",
			proposedNotional: 100,
			proposedQuantity: 0.01,
			referencePrice: 61000,
			validThroughEpoch: 1,
			forecastSource: "resonance",
			forecastModel: "online",
			forecastEpoch: 1,
			calibrationCount: 1,
			expectedReturn: 0.01,
			expectedFees: 0.0001,
			expectedSpread: 0.0001,
			expectedImpact: 0.0001,
			adverseSelection: 0.0001,
			uncertainty: 0.05,
			confidence: 0.8,
			availableCapital: 1000,
			openPositions: 0,
			slotCapacity: 4,
			cause: "edge_clear",
			reason: "utility exceeds hold",
		},
	],
	lifecycle: { "BTC/USD": "managing" },
	tradeJournal: [
		{
			kind: "lifecycle_transition",
			symbol: "BTC/USD",
			status: "entered",
			at: "2026-01-01T00:00:01Z",
			decision: 0,
		},
	],
	findings: [
		{
			symbol: "BTC/USD",
			component: "forecast",
			condition: "expected return overstated",
			evidence: ["BTC/USD realized below forecast"],
			estimatedEffect: -0.004,
			uncertainty: 0.001,
			requiredValidation: "replay on next cohort",
		},
	],
};

vi.stubGlobal("WebSocket", MockWebSocket);
WsFeed();

describe("WsFeed", () => {
	bench("applies a named backend UI frame", () => {
		applyFramePayload(frame);
	});
});
