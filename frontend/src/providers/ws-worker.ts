/// <reference lib="webworker" />

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;
const WIRE_VERSION = 1;

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "DISCONNECT" };

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DRAW"; frame: Record<string, unknown> }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> }
	| { type: "ERROR"; message: string };

const REPLACEABLE_KEYS = new Set([
	"balances",
	"executions",
	"instruments",
	"positions",
	"tick",
	"holdings",
	"stops",
	"measurements",
	"decisions",
	"lifecycle",
	"findings",
	"causal",
	"resonance",
	"manifold",
	"cognition",
	"diagnostics",
]);

let socket: WebSocket | null = null;
let socketListeners: AbortController | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";
let pending: Record<string, unknown> = {};
let rafHandle: number | null = null;

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

/*
validateFrame accepts versioned envelopes or legacy flat frames and returns the
payload object painters consume. Incompatible versions are rejected at ingress.
*/
const validateFrame = (
	raw: Record<string, unknown>,
): Record<string, unknown> | null => {
	if (typeof raw.v === "number") {
		if (raw.v !== WIRE_VERSION) {
			throw new Error(`wire version ${raw.v} incompatible with ${WIRE_VERSION}`);
		}

		const payload = raw.payload;

		if (payload === null || typeof payload !== "object" || Array.isArray(payload)) {
			throw new Error("wire payload must be an object");
		}

		return payload as Record<string, unknown>;
	}

	return raw;
};

const flushPending = () => {
	rafHandle = null;

	const frame = pending;
	pending = {};

	if (Object.keys(frame).length === 0) {
		return;
	}

	self.postMessage({
		type: "DRAW",
		frame,
	} satisfies WorkerOutbound);
};

const scheduleFlush = () => {
	if (rafHandle !== null) {
		return;
	}

	rafHandle = self.requestAnimationFrame(flushPending);
};

const coalesceFrame = (frame: Record<string, unknown>) => {
	for (const [key, value] of Object.entries(frame)) {
		if (REPLACEABLE_KEYS.has(key) || key === "error") {
			pending[key] = value;
			continue;
		}

		pending[key] = value;
	}

	scheduleFlush();
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

	socket.addEventListener(
		"open",
		() => {
			attempt = 0;

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
				const parsed = JSON.parse(String(event.data)) as Record<
					string,
					unknown
				>;
				const frame = validateFrame(parsed);

				if (frame === null) {
					return;
				}

				if (
					frame.error !== undefined &&
					frame.error !== null &&
					typeof frame.error === "object"
				) {
					self.postMessage({
						type: "ERROR_FRAME",
						frame: frame.error as Record<string, unknown>,
					} satisfies WorkerOutbound);
					return;
				}

				coalesceFrame(frame);
			} catch (err) {
				self.postMessage({
					type: "ERROR",
					message: err instanceof Error ? err.message : String(err),
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

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}

			if (rafHandle !== null) {
				self.cancelAnimationFrame(rafHandle);
				rafHandle = null;
			}

			pending = {};
			teardownSocket();
			self.postMessage({
				type: "ONLINE",
				online: false,
			} satisfies WorkerOutbound);
			return;
		}

		default: {
			const _exhaustive: never = message;
			return _exhaustive;
		}
	}
});

self.postMessage({ type: "READY" } satisfies WorkerOutbound);
