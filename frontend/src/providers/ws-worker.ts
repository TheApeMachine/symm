/// <reference lib="webworker" />

import { FrameBatcher } from "#/providers/ws-batch";
import { isPlainObject } from "#/providers/ws-frame-merge";
import {
	applyFramePayload,
	frameStores,
	subscribe,
} from "#/providers/ws-stores";
import type { Subscription } from "@tanstack/store";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "DISCONNECT" }
	| { type: "SUBSCRIBE"; id: string; store: string; key: string }
	| { type: "UNSUBSCRIBE"; id: string };

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "SUBSCRIBED"; id: string; store: string; key: string }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let socket: WebSocket | null = null;
let batcher: FrameBatcher | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";

const watches = new Map<
	string,
	{ port: MessagePort; unsubscribe: () => void }
>();

const post = (message: WorkerOutbound, transfer?: Transferable[]) => {
	self.postMessage(message, transfer ?? []);
};

const disposeBatcher = () => {
	batcher?.dispose();
	batcher = null;
};

const clearWatches = () => {
	for (const watch of watches.values()) {
		watch.unsubscribe();
		watch.port.close();
	}

	watches.clear();
};

const scheduleReconnect = () => {
	if (reconnectTimer !== null || activeUrl === "") {
		return;
	}

	const cappedDelay = Math.min(
		RECONNECT_MAX_MS,
		RECONNECT_BASE_MS * 2 ** attempt,
	);
	attempt += 1;

	const delay = Math.floor(Math.random() * cappedDelay);

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		connect(activeUrl);
	}, delay);
};

const connect = (url: string) => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	activeUrl = url;

	if (socket) {
		socket.onopen = null;
		socket.onclose = null;
		socket.onerror = null;
		socket.onmessage = null;

		if (
			socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING
		) {
			socket.close();
		}
	}

	disposeBatcher();

	batcher = new FrameBatcher((payload) => {
		const error = applyFramePayload(payload);

		if (error !== null) {
			post({ type: "ERROR_FRAME", frame: error });
		}
	});

	const currentSocket = new WebSocket(url);
	socket = currentSocket;

	currentSocket.addEventListener("open", () => {
		if (socket !== currentSocket) {
			return;
		}

		attempt = 0;
		post({ type: "ONLINE", online: true });
	});

	currentSocket.addEventListener("close", () => {
		if (socket !== currentSocket) {
			return;
		}

		post({ type: "ONLINE", online: false });
		scheduleReconnect();
	});

	currentSocket.addEventListener("error", () => {
		if (socket !== currentSocket) {
			return;
		}

		if (currentSocket.readyState === WebSocket.OPEN) {
			currentSocket.close();
		}
	});

	currentSocket.addEventListener("message", (event) => {
		if (socket !== currentSocket || batcher === null) {
			return;
		}

		try {
			const parsedData: unknown = JSON.parse(String(event.data));

			if (!isPlainObject(parsedData)) {
				throw new Error("websocket frame must be a non-null object");
			}

			batcher.enqueue(parsedData);
		} catch (err) {
			post({
				type: "ERROR",
				message: err instanceof Error ? err.message : String(err),
			});
		}
	});
};

const disconnect = () => {
	activeUrl = "";

	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	if (socket) {
		socket.onopen = null;
		socket.onclose = null;
		socket.onerror = null;
		socket.onmessage = null;

		if (
			socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING
		) {
			socket.close();
		}
	}

	socket = null;
	disposeBatcher();
	clearWatches();
	post({ type: "ONLINE", online: false });
};

const hasValues = (node: unknown): node is { values: () => unknown[] } =>
	node !== null &&
	typeof node === "object" &&
	"values" in node &&
	typeof (node as { values: unknown }).values === "function";

/*
pickRows resolves a flat buffer, a Depth1 leaf, a Depth2 outer key (full inner
histories flattened), or an empty key (latest row per leaf across Depth1/2).
*/
const pickRows = (
	root: unknown,
	key: string,
): { values: () => unknown[] } | undefined => {
	if (root === undefined || root === null || typeof root !== "object") {
		return undefined;
	}

	if (hasValues(root)) {
		return root;
	}

	const nested = root as Record<string, unknown>;

	if (key === "") {
		return {
			values: () =>
				Object.keys(nested)
					.sort()
					.flatMap((outerKey) => {
						const entry = nested[outerKey];

						if (hasValues(entry)) {
							return entry.values().slice(-1);
						}

						if (entry === null || typeof entry !== "object") {
							return [];
						}

						const inner = entry as Record<string, unknown>;

						return Object.keys(inner)
							.sort()
							.flatMap((innerKey) => {
								const leaf = inner[innerKey];

								return hasValues(leaf) ? leaf.values().slice(-1) : [];
							});
					}),
		};
	}

	const entry = nested[key];

	if (entry === undefined || entry === null || typeof entry !== "object") {
		return undefined;
	}

	if (hasValues(entry)) {
		return entry;
	}

	const inner = entry as Record<string, unknown>;

	return {
		values: () =>
			Object.keys(inner)
				.sort()
				.flatMap((innerKey) => {
					const leaf = inner[innerKey];

					return hasValues(leaf) ? leaf.values() : [];
				}),
	};
};

const subscribeTo = (id: string, store: string, key: string) => {
	const existing = watches.get(id);

	if (existing !== undefined) {
		existing.unsubscribe();
		existing.port.close();
		watches.delete(id);
	}

	const frameStore = frameStores[store as keyof typeof frameStores];

	if (frameStore === undefined) {
		return;
	}

	const { port1, port2 } = new MessageChannel();

	const pick = (state: Record<string, unknown>) =>
		pickRows(state[store], key);

	const sub = subscribe(
		frameStore as {
			subscribe: (
				listener: (state: Record<string, unknown>) => void,
			) => Subscription;
		},
		pick,
		(rows) => port1.postMessage(rows),
	);

	watches.set(id, { port: port1, unsubscribe: sub.unsubscribe });
	post({ type: "SUBSCRIBED", id, store, key }, [port2]);

	// Deliver the current snapshot after the main thread attaches port.onmessage.
	queueMicrotask(() => {
		port1.postMessage(
			pick(frameStore.state as Record<string, unknown>)?.values() ?? [],
		);
	});
};

const unsubscribe = (id: string) => {
	const watch = watches.get(id);

	if (watch === undefined) {
		return;
	}

	watch.unsubscribe();
	watch.port.close();
	watches.delete(id);
};

self.addEventListener("message", (event: MessageEvent<WorkerInbound>) => {
	const message = event.data;

	if (message.type === "CONNECT") {
		connect(message.url);
		return;
	}

	if (message.type === "DISCONNECT") {
		disconnect();
		return;
	}

	if (message.type === "SUBSCRIBE") {
		subscribeTo(message.id, message.store, message.key);
		return;
	}

	if (message.type === "UNSUBSCRIBE") {
		unsubscribe(message.id);
	}
});

post({ type: "READY" });
