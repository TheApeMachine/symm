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
	sent: string[] = [];

	constructor(_url: string) {
		MockWebSocket.latest = this;
	}

	addEventListener(type: string, listener: EventListener) {
		this.listeners.set(type, listener);
	}

	close() {
		this.readyState = 3;
	}

	send(message: string) {
		this.sent.push(message);
	}

	async emit(type: string, data: unknown) {
		await this.listeners.get(type)?.({ data } as MessageEvent);
	}
}

const encodeFrameBatch = (frames: Record<string, unknown>[]): ArrayBuffer => {
	const encoder = new TextEncoder();
	const encoded = frames.map((frame) =>
		encoder.encode(JSON.stringify(frame)),
	);
	const size = encoded.reduce(
		(total, frame) => total + Uint32Array.BYTES_PER_ELEMENT + frame.byteLength,
		Uint32Array.BYTES_PER_ELEMENT,
	);
	const payload = new ArrayBuffer(size);
	const bytes = new Uint8Array(payload);
	const view = new DataView(payload);
	let offset = 0;

	view.setUint32(offset, encoded.length, true);
	offset += Uint32Array.BYTES_PER_ELEMENT;

	for (const frame of encoded) {
		view.setUint32(offset, frame.byteLength, true);
		offset += Uint32Array.BYTES_PER_ELEMENT;
		bytes.set(frame, offset);
		offset += frame.byteLength;
	}

	return payload;
};

describe("ws-worker", () => {
	beforeEach(() => {
		vi.resetModules();
		MockWebSocket.latest = null;
	});

	it("batches every websocket frame in order for the paint thread", async () => {
		vi.useFakeTimers();
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
				type: "DRAW_BATCH",
				frames: [
					{
						resonance: [{ symbol: "BTC/USD", confidence: 0.5 }],
						cognition: { "BTC/USD": { regime: "coil" } },
					},
				],
				acknowledgeBackend: false,
			},
		]);

		await scope.emit("message", {
			type: "PAINTED",
			acknowledgeBackend: false,
		});
		expect(scope.messages[2]).toEqual({
			type: "DRAW_BATCH",
			frames: [
				{
					resonance: [{ symbol: "ETH/USD", confidence: 0.25 }],
					cognition: { "ETH/USD": { regime: "lift" } },
				},
				{
					resonance: [{ symbol: "BTC/USD", confidence: 0.75 }],
					cognition: { "BTC/USD": { regime: "break" } },
				},
			],
			acknowledgeBackend: false,
		});

		await socket?.emit(
			"message",
			JSON.stringify({ measurements: { epoch: 4 } }),
		);
		expect(scope.messages).toHaveLength(3);

		await scope.emit("message", {
			type: "PAINTED",
			acknowledgeBackend: false,
		});
		expect(scope.messages).toHaveLength(4);
		expect(scope.messages[3]).toEqual({
			type: "DRAW_BATCH",
			frames: [{ measurements: { epoch: 4 } }],
			acknowledgeBackend: false,
		});
	});

	it("decodes every frame from a binary backend batch", async () => {
		vi.useFakeTimers();
		const scope = new MockWorkerScope();

		vi.stubGlobal("self", scope);
		vi.stubGlobal("WebSocket", MockWebSocket);
		await import("#/providers/ws-worker");
		await scope.emit("message", {
			type: "CONNECT",
			url: "ws://127.0.0.1:8765/ws",
		});

		const frames = [
			{ measurements: { tick: 1 } },
			{ measurements: { tick: 2 } },
			{ measurements: { tick: 3 } },
		];
		await MockWebSocket.latest?.emit("message", encodeFrameBatch(frames));
		expect(scope.messages).toEqual([
			{ type: "READY" },
			{ type: "DRAW_BATCH", frames, acknowledgeBackend: true },
		]);

		if (MockWebSocket.latest !== null) {
			MockWebSocket.latest.readyState = MockWebSocket.OPEN;
		}

		await scope.emit("message", {
			type: "PAINTED",
			acknowledgeBackend: true,
		});
		expect(MockWebSocket.latest?.sent).toEqual([
			JSON.stringify({ type: "painted" }),
		]);
	});
});
