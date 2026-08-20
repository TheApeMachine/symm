import * as flatbuffers from "flatbuffers";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BatchT } from "#/providers/telemetry/telemetry/batch";
import { FrameEntryT } from "#/providers/telemetry/telemetry/frame-entry";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { TickFrameT } from "#/providers/telemetry/telemetry/tick-frame";

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

const encodeTick = (count: number): FrameEntryT =>
	new FrameEntryT(Frame.TickFrame, new TickFrameT(BigInt(count)));

const encodeFrameBatch = (counts: number[]): ArrayBuffer => {
	const builder = new flatbuffers.Builder(64);
	const offset = new BatchT(
		BigInt(counts.at(-1) ?? 0),
		counts.map(encodeTick),
	).pack(builder);
	builder.finish(offset, "SYMB");
	const bytes = builder.asUint8Array();
	return bytes.buffer.slice(
		bytes.byteOffset,
		bytes.byteOffset + bytes.byteLength,
	) as ArrayBuffer;
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

	it("advances ingestion while presenting only the newest state", async () => {
		const { scope, socket } = await connectWorker();

		await socket.emit("message", encodeFrameBatch([1]));
		await socket.emit("message", encodeFrameBatch([2]));
		await socket.emit("message", encodeFrameBatch([3]));

		expect(scope.messages).toEqual([
			{ type: "READY" },
			{ type: "DRAW_BATCH", frames: [{ tick: { count: 1 } }] },
		]);
		expect(socket.sent).toEqual([
			Uint8Array.of(1),
			Uint8Array.of(1),
			Uint8Array.of(1),
		]);

		await scope.emit("message", { type: "PAINTED", acknowledgeBackend: false });
		expect(scope.messages.at(-1)).toEqual({
			type: "DRAW_BATCH",
			frames: [{ tick: { count: 3 } }],
		});

		await scope.emit("message", { type: "PAINTED", acknowledgeBackend: false });
		expect(scope.messages).toHaveLength(3);
		expect(socket.sent).toEqual([
			Uint8Array.of(1),
			Uint8Array.of(1),
			Uint8Array.of(1),
		]);
	});

	it("coalesces repeated state tables from one backend batch", async () => {
		const { scope, socket } = await connectWorker();

		await socket.emit("message", encodeFrameBatch([1, 2, 3]));
		expect(scope.messages).toEqual([
			{ type: "READY" },
			{
				type: "DRAW_BATCH",
				frames: [{ tick: { count: 3 } }],
			},
		]);
		expect(socket.sent).toEqual([Uint8Array.of(1)]);
	});
});
