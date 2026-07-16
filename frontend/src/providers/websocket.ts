import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { FrameBatcher } from "#/providers/ws-batch";
import { applyFramePayload } from "#/providers/ws-stores";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;
const useWorkerTransport = import.meta.env.VITEST !== "true";

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DATA_UPDATE"; payload: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let worker: Worker | null = null;
let socket: WebSocket | null = null;
let batcher: FrameBatcher | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let lastProjection = "";

const sendProjection = (force = false) => {
	const projection = {
		focusSymbol: appStore.state.focusSymbol,
		source: terminalStore.state.selectedSource,
	};
	const key = `${projection.focusSymbol}\u0000${projection.source}`;

	if (!force && key === lastProjection) {
		return;
	}

	if (worker !== null) {
		worker.postMessage({ type: "CONTROL", projection });
		lastProjection = key;
		return;
	}

	if (socket?.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify(projection));
		lastProjection = key;
	}
};

const disconnectTransport = () => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	if (worker !== null) {
		worker.postMessage({ type: "DISCONNECT" });
		worker.terminate();
		worker = null;
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

		socket = null;
	}

	batcher?.dispose();
	batcher = null;
};

const scheduleReconnect = (connect: () => void) => {
	if (reconnectTimer !== null) {
		return;
	}

	const delay = Math.min(RECONNECT_MAX_MS, RECONNECT_BASE_MS * 2 ** attempt);
	attempt += 1;

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		connect();
	}, delay);
};

const handleWorkerMessage = (event: MessageEvent<WorkerOutbound>) => {
	const message = event.data;

	if (message.type === "READY") {
		worker?.postMessage({ type: "CONNECT", url: socketUrl });
		return;
	}

	if (message.type === "ONLINE") {
		if (message.online) {
			sendProjection(true);
		}

		appStore.actions.updateOnline(message.online);

		return;
	}

	if (message.type === "DATA_UPDATE") {
		applyFramePayload(message.payload);
		return;
	}

	if (message.type === "ERROR") {
		appStore.actions.updateError({ message: message.message });
	}
};

const connectWorker = () => {
	disconnectTransport();

	worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
		type: "module",
	});

	worker.addEventListener("message", handleWorkerMessage);
	worker.addEventListener("error", (event) => {
		console.error("WS worker failed:", event.message);
		appStore.actions.updateOnline(false);
		appStore.actions.updateError({ message: event.message });
	});
};

const connectInline = () => {
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

	batcher?.dispose();
	batcher = new FrameBatcher(applyFramePayload);

	const currentSocket = new WebSocket(socketUrl);
	socket = currentSocket;

	currentSocket.addEventListener("open", () => {
		if (socket !== currentSocket) {
			return;
		}

		attempt = 0;
		sendProjection(true);
		appStore.actions.updateOnline(true);
	});

	currentSocket.addEventListener("close", () => {
		if (socket !== currentSocket) {
			return;
		}

		appStore.actions.updateOnline(false);
		scheduleReconnect(connectInline);
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
			const parsedData = JSON.parse(String(event.data)) as Record<
				string,
				unknown
			>;

			batcher.enqueue(parsedData);
		} catch (err) {
			console.error("WS Parse Error:", err);
			appStore.actions.updateError({ err });
		}
	});
};

const connect = () => {
	if (useWorkerTransport) {
		connectWorker();
		return;
	}

	connectInline();
};

/*
WsFeed boots the websocket transport once and keeps backend frames flowing into
the TanStack stores through either the 16ms worker batcher or the inline path.
*/
export const WsFeed = () => {
	useEffect(() => {
		connect();
		const focusSubscription = appStore.subscribe(() => sendProjection());
		const sourceSubscription = terminalStore.subscribe(() => sendProjection());

		return () => {
			focusSubscription.unsubscribe();
			sourceSubscription.unsubscribe();
			disconnectTransport();
		};
	}, []);

	return null;
};

export { applyFramePayload };
