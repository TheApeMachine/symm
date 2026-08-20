/// <reference lib="webworker" />

const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 5000;

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "DISCONNECT" }
	| { type: "PAINTED"; acknowledgeBackend: boolean }
	| { type: "FOCUS"; symbol: string }
	| { type: "BACKTEST"; action: "play" | "pause" | "seek" | "select" | "hindsight"; at?: string; captureId?: number };

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DRAW"; frame: Record<string, unknown> }
	| {
			type: "DRAW_BATCH";
			batches: ArrayBuffer[];
			acknowledgeBackend: boolean;
	  }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let socket: WebSocket | null = null;
let socketListeners: AbortController | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";
let focusSymbol = "";
let pendingBatches: ArrayBuffer[] = [];
let paintInFlight = false;
let backendAckPending = false;

/*
flushBatches transfers every binary telemetry batch to the main thread without
copying or decoding it in the worker. The main thread decodes the bytes inside
the paint callback, so no JavaScript telemetry object crosses this boundary.
*/
const flushBatches = () => {
	if (paintInFlight || pendingBatches.length === 0) {
		return;
	}

	const batches = pendingBatches;
	const acknowledgeBackend = backendAckPending;
	pendingBatches = [];
	backendAckPending = false;
	paintInFlight = true;
	self.postMessage(
		{
			type: "DRAW_BATCH",
			batches,
			acknowledgeBackend,
		} satisfies WorkerOutbound,
		batches,
	);
};

const stopFrameFlush = () => {
	pendingBatches = [];
	paintInFlight = false;
	backendAckPending = false;
};

/*
sendBacktest pushes one playback command to the backend driver. No-ops until
the socket is open.
*/
const sendBacktest = (
	action: "play" | "pause" | "seek" | "select" | "hindsight",
	at?: string,
	captureId?: number,
) => {
	if (socket === null || socket.readyState !== WebSocket.OPEN) {
		return;
	}

	socket.send(
		JSON.stringify({ type: `backtest.${action}`, at, captureId }),
	);
};

/*
sendFocus pushes the dashboard focus to the backend so signal-metric publishes
can gate on the selected symbol. No-ops until the socket is open.
*/
const sendFocus = (symbol: string) => {
	focusSymbol = symbol;

	if (socket === null || socket.readyState !== WebSocket.OPEN) {
		return;
	}

	socket.send(JSON.stringify({ type: "focus", symbol }));
};

/*
teardownSocket aborts the current connection's listeners (one AbortController per
socket removes them all) and closes the socket, so a superseded connection can
neither fire late events nor leak listeners.
*/
const teardownSocket = () => {
	socketListeners?.abort();
	socketListeners = null;

	if (
		socket &&
		(socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING)
	) {
		socket.close();
	}

	socket = null;
};

const connect = (url: string) => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	activeUrl = url;
	teardownSocket();

	socketListeners = new AbortController();
	socket = new WebSocket(url);
	socket.binaryType = "arraybuffer";

	socket.addEventListener(
		"open",
		() => {
			attempt = 0;

			if (focusSymbol !== "") {
				sendFocus(focusSymbol);
			}

			self.postMessage({
				type: "ONLINE",
				online: true,
			} satisfies WorkerOutbound);
		},
		{ signal: socketListeners.signal },
	);

	socket.addEventListener(
		"close",
		() => {
			self.postMessage({
				type: "ONLINE",
				online: false,
			} satisfies WorkerOutbound);

			if (reconnectTimer !== null || activeUrl === "") {
				return;
			}

			const cappedDelay = Math.min(
				RECONNECT_MAX_MS,
				RECONNECT_BASE_MS * 2 ** attempt,
			);
			attempt += 1;

			reconnectTimer = setTimeout(
				() => {
					reconnectTimer = null;
					connect(activeUrl);
				},
				Math.floor(Math.random() * cappedDelay),
			);
		},
		{ signal: socketListeners.signal },
	);

	socket.addEventListener(
		"error",
		(event) => {
			if (
				event.currentTarget instanceof WebSocket &&
				event.currentTarget.readyState === WebSocket.OPEN
			) {
				event.currentTarget.close();
			}
		},
		{ signal: socketListeners.signal },
	);

	socket.addEventListener(
		"message",
		(event) => {
			try {
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error("telemetry websocket requires a binary frame");
				}

				pendingBatches.push(event.data);
				backendAckPending = true;
				flushBatches();
			} catch (err) {
				console.log(event);

				self.postMessage({
					type: "ERROR",
					message:
						err instanceof Error
							? `${err.message}\n\n${err.stack}\n\n${event.data}`
							: String(err),
				} satisfies WorkerOutbound);
			}
		},
		{ signal: socketListeners.signal },
	);
};

self.addEventListener("message", (event: MessageEvent<WorkerInbound>) => {
	const message = event.data;

	switch (message.type) {
		case "CONNECT": {
			connect(message.url);
			return;
		}

		case "DISCONNECT": {
			activeUrl = "";
			stopFrameFlush();

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}

			teardownSocket();
			self.postMessage({
				type: "ONLINE",
				online: false,
			} satisfies WorkerOutbound);
			return;
		}

		case "PAINTED": {
			paintInFlight = false;

			if (
				message.acknowledgeBackend &&
				socket !== null &&
				socket.readyState === WebSocket.OPEN
			) {
				socket.send(JSON.stringify({ type: "painted" }));
			}

			flushBatches();
			return;
		}

		case "FOCUS": {
			sendFocus(message.symbol);
			return;
		}

		case "BACKTEST": {
			sendBacktest(message.action, message.at, message.captureId);
			return;
		}

		default: {
			const _exhaustive: never = message;
			return _exhaustive;
		}
	}
});

self.postMessage({ type: "READY" } satisfies WorkerOutbound);

export {};
