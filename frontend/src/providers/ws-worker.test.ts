import { beforeEach, describe, expect, it, vi } from "vitest";

type EventListener = (event: MessageEvent) => void | Promise<void>;

class MockWorkerScope {
	listeners = new Map<string, EventListener>();
	messages: unknown[] = [];

	addEventListener(type: string, listener: EventListener) {
		this.listeners.set(type, listener);
	}

	postMessage(message: unknown) {
		this.messages.push(message);
	}

	async emit(type: string, data: unknown) {
		await this.listeners.get(type)?.({ data } as MessageEvent);
	}
}

class MockWebSocket {
	static OPEN = 1;
	static CONNECTING = 0;
	static latest: MockWebSocket | null = null;

	readyState = MockWebSocket.CONNECTING;
	binaryType = "";
	listeners = new Map<string, EventListener>();

	constructor(_url: string) {
		MockWebSocket.latest = this;
	}

	addEventListener(type: string, listener: EventListener) {
		this.listeners.set(type, listener);
	}

	close() {
		this.readyState = 3;
	}

	send(_message: string) {}

	async emit(type: string, data: unknown) {
		await this.listeners.get(type)?.({ data } as MessageEvent);
	}
}

describe("ws-worker", () => {
	beforeEach(() => {
		vi.resetModules();
		MockWebSocket.latest = null;
	});

	it("holds and merges draw deltas until the main thread acknowledges its paint", async () => {
		const scope = new MockWorkerScope();

		vi.stubGlobal("self", scope);
		vi.stubGlobal("WebSocket", MockWebSocket);
		await import("#/providers/ws-worker");
		await scope.emit("message", {
			type: "CONNECT",
			url: "ws://127.0.0.1:8765/ws",
		});

		const socket = MockWebSocket.latest;

		expect(socket).not.toBeNull();
		await socket?.emit(
			"message",
			JSON.stringify({
				resonance: [{ symbol: "BTC/USD", confidence: 0.5 }],
				cognition: { "BTC/USD": { regime: "coil" } },
			}),
		);
		await socket?.emit(
			"message",
			JSON.stringify({
				resonance: [{ symbol: "ETH/USD", confidence: 0.25 }],
				cognition: { "ETH/USD": { regime: "lift" } },
			}),
		);
		await socket?.emit(
			"message",
			JSON.stringify({
				resonance: [{ symbol: "BTC/USD", confidence: 0.75 }],
				cognition: { "BTC/USD": { regime: "break" } },
			}),
		);

		expect(scope.messages).toEqual([
			{ type: "READY" },
			{
				type: "DRAW",
				frame: {
					resonance: [{ symbol: "BTC/USD", confidence: 0.5 }],
					cognition: { "BTC/USD": { regime: "coil" } },
				},
			},
		]);

		await scope.emit("message", { type: "DRAWN" });

		expect(scope.messages.at(-1)).toEqual({
			type: "DRAW",
			frame: {
				resonance: [
					{ symbol: "ETH/USD", confidence: 0.25 },
					{ symbol: "BTC/USD", confidence: 0.75 },
				],
				cognition: {
					"ETH/USD": { regime: "lift" },
					"BTC/USD": { regime: "break" },
				},
			},
		});
	});
});
