import { afterEach, describe, expect, it, vi } from "vitest";

import { subscribeSymmWsFeed } from "#/providers/symm-ws-client";

class MockWebSocket {
	static instances: MockWebSocket[] = [];
	static deferCloseEvent = false;

	readyState = WebSocket.CONNECTING;
	binaryType = "blob";

	private listeners: Record<string, Set<EventListener>> = {
		open: new Set(),
		close: new Set(),
		message: new Set(),
	};

	constructor(url: string | URL) {
		this.url = String(url);
		MockWebSocket.instances.push(this);

		queueMicrotask(() => {
			this.readyState = WebSocket.OPEN;
			this.emit("open", new Event("open"));
		});
	}

	url: string;

	addEventListener = (type: string, listener: EventListener) => {
		this.listeners[type]?.add(listener);
	};

	close = () => {
		this.readyState = WebSocket.CLOSED;

		if (MockWebSocket.deferCloseEvent) {
			return;
		}

		this.emit("close", new Event("close"));
	};

	emit(type: string, event: Event) {
		for (const listener of this.listeners[type] ?? []) {
			listener(event);
		}
	}
}

describe("subscribeSymmWsFeed", () => {
	afterEach(async () => {
		await new Promise((resolve) => setTimeout(resolve, 200));

		MockWebSocket.instances = [];
		MockWebSocket.deferCloseEvent = false;
		vi.unstubAllGlobals();
	});

	it("reuses one socket across strict-mode style remounts", async () => {
		vi.stubGlobal("WebSocket", MockWebSocket);

		const onMessage = vi.fn();
		const onConnection = vi.fn();

		const unsubscribeFirst = subscribeSymmWsFeed(
			"ws://127.0.0.1:8765/ws",
			onMessage,
			onConnection,
		);

		expect(MockWebSocket.instances.length).toBe(1);

		unsubscribeFirst();

		const unsubscribeSecond = subscribeSymmWsFeed(
			"ws://127.0.0.1:8765/ws",
			onMessage,
			onConnection,
		);

		expect(MockWebSocket.instances.length).toBe(1);

		unsubscribeSecond();

		await new Promise((resolve) => setTimeout(resolve, 200));

		const unsubscribeThird = subscribeSymmWsFeed(
			"ws://127.0.0.1:8765/ws",
			onMessage,
			onConnection,
		);

		expect(MockWebSocket.instances.length).toBe(2);

		unsubscribeThird();
	});

	it("ignores close events from replaced sockets", async () => {
		vi.stubGlobal("WebSocket", MockWebSocket);
		MockWebSocket.deferCloseEvent = true;

		const onMessage = vi.fn();
		const onConnection = vi.fn();

		const unsubscribeFirst = subscribeSymmWsFeed(
			"ws://127.0.0.1:8765/ws",
			onMessage,
			onConnection,
		);

		await new Promise((resolve) => queueMicrotask(resolve));

		unsubscribeFirst();

		await new Promise((resolve) => setTimeout(resolve, 200));

		const staleSocket = MockWebSocket.instances[0];
		const unsubscribeSecond = subscribeSymmWsFeed(
			"ws://127.0.0.1:8765/ws",
			onMessage,
			onConnection,
		);

		await new Promise((resolve) => queueMicrotask(resolve));

		staleSocket?.emit("close", new Event("close"));

		expect(onConnection).toHaveBeenLastCalledWith(true);

		unsubscribeSecond();
	});
});
