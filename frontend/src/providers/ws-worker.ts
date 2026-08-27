/// <reference lib="webworker" />

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

const sendBacktest = (
	action: "play" | "pause" | "seek" | "select" | "hindsight",
	at?: string,
	captureId?: number,
) => {
	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify({ type: `backtest.${action}`, at, captureId }));
	}
};

const sendPositionExit = (symbol: string) => {
	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify({ type: "position.exit", symbol }));
	}
};

const sendFocus = (symbol: string) => {
	if (socket !== null && socket.readyState === WebSocket.OPEN && symbol) {
		socket.send(JSON.stringify({ type: "focus", symbol }));
	}
};

const teardownSocket = () => {
	// Invalidate the current socket before an intentional close so its stale
	// lifecycle handlers can never schedule a reconnect for a replaced or
	// deliberately disconnected connection.
	socketGeneration += 1;

	if (
		socket !== null &&
		(socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING)
	) {
		socket.close();
	}

	socket = null;
};

const scheduleReconnect = () => {
	if (reconnectTimer !== null) {
		return;
	}

	const delay = Math.min(
		RECONNECT_BASE_MS * 2 ** reconnectAttempts,
		RECONNECT_MAX_MS,
	);

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		reconnectAttempts += 1;
		connect(connectUrl);
	}, delay);
};

const connect = (url: string) => {
	if (url === "") {
		return;
	}

	teardownSocket();

	connectUrl = url;
	const generation = socketGeneration;
	socket = new WebSocket(url);
	socket.binaryType = "arraybuffer";

	socket.addEventListener("open", () => {
		if (generation !== socketGeneration) {
			return;
		}

		reconnectAttempts = 0;
		self.postMessage({ type: "STATUS", status: "ONLINE" });
	});

	socket.addEventListener("close", () => {
		if (generation !== socketGeneration) {
			return;
		}

		self.postMessage({ type: "STATUS", status: "OFFLINE" });
		scheduleReconnect();
	});

	socket.addEventListener("error", (event) => {
		if (generation !== socketGeneration) {
			return;
		}

		self.postMessage({ type: "ERROR", error: String(event) });
	});

	socket.addEventListener("message", (event) => {
		if (generation !== socketGeneration) {
			return;
		}

		try {
			const raw = event.data;
			let arrayBuffer: ArrayBuffer | null = null;

			if (raw instanceof ArrayBuffer) {
				arrayBuffer = raw;
			} else if (raw instanceof Uint8Array) {
				arrayBuffer = raw.buffer.slice(
					raw.byteOffset,
					raw.byteOffset + raw.byteLength,
				) as ArrayBuffer;
			} else if (raw?.buffer instanceof ArrayBuffer) {
				arrayBuffer = raw.buffer.slice(
					raw.byteOffset,
					raw.byteOffset + raw.byteLength,
				) as ArrayBuffer;
			}

			if (arrayBuffer) {
				self.postMessage({ type: "BATCH", buffer: arrayBuffer }, [arrayBuffer]);
			}
		} catch (err) {
			self.postMessage({ type: "ERROR", error: String(err) });
		}
	});
};

self.postMessage({ type: "READY" });

self.addEventListener("message", (event: MessageEvent) => {
	const message = event.data as {
		type: string;
		url?: string;
		action?: "play" | "pause" | "seek" | "select" | "hindsight";
		at?: string;
		captureId?: number;
		symbol?: string;
	};

	switch (message.type) {
		case "CONNECT":
			connect(message.url ?? "");
			return;
		case "DISCONNECT":
			// Explicit teardown cancels any pending reconnect so a deliberate
			// disconnect (unmount, teardown) never spins the backoff loop.
			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}

			reconnectAttempts = 0;
			teardownSocket();
			return;
		case "FOCUS":
			sendFocus(message.symbol ?? "");
			return;
		case "BACKTEST":
			sendBacktest(message.action ?? "play", message.at, message.captureId);
			return;
		case "POSITION_EXIT":
			sendPositionExit(message.symbol ?? "");
			return;
	}
});

export {};
