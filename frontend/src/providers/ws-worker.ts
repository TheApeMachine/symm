/// <reference lib="webworker" />

import { atoms, signals } from "#/collections/app";
import { RingBuffer } from "#/collections/ring";
import { parseValues } from "#/types/decoder";
import { Value } from "#/types/value";

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

let currentRoute = "dashboard";
let pendingCommands: Record<string, unknown>[] = [];

const queueCommand = (command: Record<string, unknown>) => {
	if (command.type === "hindsight") {
		pendingCommands = pendingCommands.filter(
			(item) => item.type !== "hindsight",
		);
	}

	pendingCommands.push(command);
};

const flushCommands = () => {
	if (socket === null || socket.readyState !== WebSocket.OPEN) {
		return;
	}

	for (const command of pendingCommands) {
		socket.send(JSON.stringify(command));
	}
};

const sendRoute = (route: string) => {
	currentRoute = route;
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof route === "string"
	) {
		socket.send(JSON.stringify({ type: "route", route }));
	}
};

const sendFocus = (symbol: string) => {
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof symbol === "string"
	) {
		socket.send(JSON.stringify({ type: "focus", symbol }));
	}
};

const sendPositionExit = (symbol: string) => {
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof symbol === "string"
	) {
		socket.send(JSON.stringify({ type: "position.exit", symbol }));
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
		if (currentRoute) {
			sendRoute(currentRoute);
		}
		flushCommands();
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
				const value = parseValues(new Uint8Array(arrayBuffer));
				if (value) {
					for (const val of value) {
						const origin = val.origin;

						// Route by origin: atom or signal store.
						const atom = atoms[origin];
						const store = signals[origin];

						if (atom) {
							// Values with atom origins are decoded in the main
							// thread via the Value.lookup path once they land
							// in the signals store; here we just record the
							// tick into the ring buffer if one exists.
							continue;
						}

						if (store) {
							const symbol = val.scope;
							let ring = store.state[symbol];

							if (!ring) {
								ring = new RingBuffer<Value>(50);
								store.state[symbol] = ring;
							}

							const decoded = new Value({
								id: val.id,
								epoch: val.epoch,
								tick: val.tick,
								seqIdx: val.seqIdx,
								timestamp: val.timestamp,
								origin: val.origin,
								role: val.role,
								scope: val.scope,
								layers: val.layers.map((layer) => layer.bytes),
								index: val.layers.map((layer) => layer.index),
							});

							ring.add(decoded);
						}
					}
				}
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
		symbol?: string;
		route?: string;
		command?: Record<string, unknown>;
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
			pendingCommands = [];
			teardownSocket();
			return;
		case "FOCUS":
			sendFocus(message.symbol ?? "");
			return;
		case "ROUTE":
			sendRoute(message.route ?? "");
			return;
		case "POSITION_EXIT":
			sendPositionExit(message.symbol ?? "");
			return;
		case "COMMAND": {
			const command = message.command;

			if (!command) {
				self.postMessage({
					type: "COMMAND_FAILED",
					command,
					error: "hub command is empty",
				});
				return;
			}

			queueCommand(command);

			if (socket !== null && socket.readyState === WebSocket.OPEN) {
				socket.send(JSON.stringify(command));
			}

			return;
		}
	}
});
