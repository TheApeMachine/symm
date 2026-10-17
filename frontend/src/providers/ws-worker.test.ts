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
	sent: unknown[] = [];

	constructor(_url: string) {
		MockWebSocket.latest = this;
	}

	addEventListener(type: string, listener: EventListener) {
		this.listeners.set(type, listener);
	}

	close() {
		this.readyState = 3;
	}

	send(message: unknown) {
		this.sent.push(message);
	}

	async emit(type: string, data: unknown) {
		await this.listeners.get(type)?.({ data } as MessageEvent);
	}
}

const encodeFrameBatch = (counts: number[]): ArrayBuffer => {
	const buf = new Uint8Array(counts.length);
	for (let i = 0; i < counts.length; i++) {
		buf[i] = counts[i];
	}
	return buf.buffer;
};

const connectWorker = async () => {
	const scope = new MockWorkerScope();

	vi.stubGlobal("self", scope);
	vi.stubGlobal("WebSocket", MockWebSocket);
	await import("#/providers/ws-worker");
	await scope.emit("message", {
		type: "CONNECT",
		url: "ws://127.0.0.1:8765/ws",
	});

	const socket = MockWebSocket.latest;

	if (socket === null) {
		throw new Error("worker did not create a websocket");
	}

	socket.readyState = MockWebSocket.OPEN;
	return { scope, socket };
};

describe("ws-worker", () => {
	beforeEach(() => {
		vi.resetModules();
		MockWebSocket.latest = null;
	});

	it("emits STATUS online on open and forwards binary batches to the main thread", async () => {
		const { scope, socket } = await connectWorker();

		// Initial READY from worker instantiation
		expect(scope.messages).toContainEqual({ type: "READY" });

		// Open socket: must notify main thread of ONLINE
		await socket.emit("open", {});
		expect(scope.messages).toContainEqual({ type: "STATUS", status: "ONLINE" });

		// Inbound batch: must transfer binary buffer to main thread via postMessage
		const batchBuffer = encodeFrameBatch([1, 2, 3]);
		await socket.emit("message", batchBuffer);

		const batchMsg = scope.messages.find(
			(m: any) => m.type === "BATCH" && m.buffer instanceof ArrayBuffer,
		) as { type: "BATCH"; buffer: ArrayBuffer } | undefined;

		expect(batchMsg).toBeDefined();
		expect(batchMsg?.buffer.byteLength).toBe(batchBuffer.byteLength);

		// Close socket: must notify main thread of OFFLINE
		await socket.emit("close", {});
		expect(scope.messages).toContainEqual({
			type: "STATUS",
			status: "OFFLINE",
		});
	});

	it("reconnects automatically after the socket closes", async () => {
		const { scope, socket } = await connectWorker();
		await socket.emit("open", {});

		// Close: OFFLINE and a reconnect must be scheduled (500ms base backoff).
		await socket.emit("close", {});

		const reconnect = new Promise<void>((resolve) => {
			const timer = setInterval(() => {
				if (MockWebSocket.latest !== null && MockWebSocket.latest !== socket) {
					clearInterval(timer);
					resolve();
				}
			}, 10);
		});

		await reconnect;
		expect(scope.messages).toContainEqual({
			type: "STATUS",
			status: "OFFLINE",
		});
	});

	it("dispatches main thread commands to the backend websocket", async () => {
		const { scope, socket } = await connectWorker();
		await socket.emit("open", {});

		await scope.emit("message", { type: "FOCUS", symbol: "SOL/USD" });
		expect(socket.sent).toContainEqual(
			JSON.stringify({ type: "focus", symbol: "SOL/USD" }),
		);

		await scope.emit("message", { type: "POSITION_EXIT", symbol: "ETH/USD" });
		expect(socket.sent).toContainEqual(
			JSON.stringify({ type: "position.exit", symbol: "ETH/USD" }),
		);
	});

	it("queues hub commands while the socket is connecting and flushes them on open", async () => {
		const scope = new MockWorkerScope();
		vi.stubGlobal("self", scope);
		vi.stubGlobal("WebSocket", MockWebSocket);
		await import("#/providers/ws-worker");
		await scope.emit("message", {
			type: "CONNECT",
			url: "ws://127.0.0.1:8765/ws",
		});

		const socket = MockWebSocket.latest;
		if (socket === null) throw new Error("worker did not create a websocket");

		const command = { type: "hindsight", run: "7", symbol: "BTC/USD" };
		await scope.emit("message", { type: "COMMAND", command });

		expect(scope.messages).not.toContainEqual(
			expect.objectContaining({ type: "COMMAND_FAILED" }),
		);
		expect(socket.sent).toEqual([]);

		socket.readyState = MockWebSocket.OPEN;
		await socket.emit("open", {});

		expect(socket.sent).toContainEqual(JSON.stringify(command));
	});

	it("resends a hindsight command after the socket reconnects", async () => {
		const { scope, socket } = await connectWorker();
		await socket.emit("open", {});

		const command = { type: "hindsight", run: "7", symbol: "BTC/USD" };
		await scope.emit("message", { type: "COMMAND", command });
		expect(socket.sent).toContainEqual(JSON.stringify(command));

		await socket.emit("close", {});

		const reconnect = new Promise<MockWebSocket>((resolve) => {
			const timer = setInterval(() => {
				if (MockWebSocket.latest !== null && MockWebSocket.latest !== socket) {
					clearInterval(timer);
					resolve(MockWebSocket.latest);
				}
			}, 10);
		});

		const next = await reconnect;
		next.readyState = MockWebSocket.OPEN;
		await next.emit("open", {});
		expect(next.sent).toContainEqual(JSON.stringify(command));
	});
});
