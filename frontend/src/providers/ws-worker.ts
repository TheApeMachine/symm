/// <reference lib="webworker" />

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;
const WIRE_VERSION = 1;

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "DISCONNECT" }
	| { type: "FOCUS"; symbol: string };

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DRAW"; frame: Record<string, unknown> }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let socket: WebSocket | null = null;
let socketListeners: AbortController | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";
let pending: Record<string, unknown> = {};
let pendingGeneration = new Map<string, number>();
const appliedGeneration = new Map<string, number>();
let rafHandle: number | null = null;
let incompatible = false;
let focusSymbol = "";

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

/*
validateFrame accepts versioned envelopes or legacy flat frames and returns the
payload object painters consume plus the envelope generation when present.
*/
const validateFrame = (
	raw: Record<string, unknown>,
): { payload: Record<string, unknown>; generation?: number } | null => {
	if (typeof raw.v === "number") {
		if (raw.v !== WIRE_VERSION) {
			throw new Error(
				`wire version ${raw.v} incompatible with ${WIRE_VERSION}`,
			);
		}

		const payload = raw.payload;

		if (
			payload === null ||
			typeof payload !== "object" ||
			Array.isArray(payload)
		) {
			throw new Error("wire payload must be an object");
		}

		const generation =
			typeof raw.g === "number" && Number.isFinite(raw.g) ? raw.g : undefined;

		return {
			payload: payload as Record<string, unknown>,
			generation,
		};
	}

	return { payload: raw };
};

const flushPending = () => {
	rafHandle = null;

	const frame = pending;
	pending = {};

	for (const [key, generation] of pendingGeneration) {
		const prior = appliedGeneration.get(key) ?? 0;

		if (generation >= prior) {
			appliedGeneration.set(key, generation);
		}
	}

	pendingGeneration = new Map();

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

/*
coalesceFrame replaces pending keys; generation-aware ordering drops frames that
would regress same-key state already applied or queued at a newer generation.
*/
const coalesceFrame = (frame: Record<string, unknown>, generation?: number) => {
	for (const [key, value] of Object.entries(frame)) {
		if (generation !== undefined) {
			const applied = appliedGeneration.get(key) ?? 0;
			const queued = pendingGeneration.get(key) ?? 0;

			if (generation < applied || generation < queued) {
				continue;
			}

			pendingGeneration.set(key, generation);
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
	incompatible = false;

	socketListeners = new AbortController();
	socket = new WebSocket(url);

	socket.addEventListener(
		"open",
		() => {
			attempt = 0;
			incompatible = false;

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

			if (reconnectTimer !== null || activeUrl === "" || incompatible) {
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
			if (incompatible) {
				return;
			}

			try {
				const parsed = JSON.parse(String(event.data)) as Record<
					string,
					unknown
				>;
				const validated = validateFrame(parsed);

				if (validated === null) {
					return;
				}

				const { payload: frame, generation } = validated;

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

				coalesceFrame(frame, generation);
			} catch (err) {
				const message = err instanceof Error ? err.message : String(err);

				if (message.includes("wire version")) {
					incompatible = true;
				}

				self.postMessage({
					type: "ERROR",
					message,
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
			incompatible = false;

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}

			if (rafHandle !== null) {
				self.cancelAnimationFrame(rafHandle);
				rafHandle = null;
			}

			pending = {};
			pendingGeneration = new Map();
			teardownSocket();
			self.postMessage({
				type: "ONLINE",
				online: false,
			} satisfies WorkerOutbound);
			return;
		}

		case "FOCUS": {
			sendFocus(message.symbol);
			return;
		}

		default: {
			const _exhaustive: never = message;
			return _exhaustive;
		}
	}
});

self.postMessage({ type: "READY" } satisfies WorkerOutbound);
