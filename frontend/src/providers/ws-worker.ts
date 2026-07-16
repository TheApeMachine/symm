/// <reference lib="webworker" />

import { FrameBatcher } from "#/providers/ws-batch";
import { isPlainObject } from "#/providers/ws-frame-merge";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

type ViewProjection = { focusSymbol: string; source: string };

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "CONTROL"; projection: ViewProjection }
	| { type: "DISCONNECT" };

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DATA_UPDATE"; payload: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let socket: WebSocket | null = null;
let batcher: FrameBatcher | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";

const post = (message: WorkerOutbound) => {
	self.postMessage(message);
};

const disposeBatcher = () => {
	batcher?.dispose();
	batcher = null;
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
		post({ type: "DATA_UPDATE", payload });
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
	post({ type: "ONLINE", online: false });
};

self.addEventListener("message", (event: MessageEvent<WorkerInbound>) => {
	const message = event.data;

	if (message.type === "CONNECT") {
		connect(message.url);
		return;
	}

	if (message.type === "CONTROL") {
		if (socket?.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify(message.projection));
		}

		return;
	}

	if (message.type === "DISCONNECT") {
		disconnect();
	}
});

post({ type: "READY" });
