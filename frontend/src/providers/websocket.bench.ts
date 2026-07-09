import { bench, describe, vi } from "vitest";
import { WsFeed } from "./websocket";

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

const frame = JSON.stringify({
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
	executions: [
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
});

vi.stubGlobal("WebSocket", MockWebSocket);
WsFeed();
const socket = MockWebSocket.instances[0];

describe("WsFeed", () => {
	bench("applies a named backend UI frame", () => {
		socket.emit("message", { data: frame });
	});
});
