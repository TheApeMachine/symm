/// <reference lib="webworker" />

import { atoms, signals } from "#/collections/app";
import { RingBuffer } from "#/collections/ring";
import { parseValues } from "#/types/decoder";
import type { Value } from "#/types/value";

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

let currentRoute = "dashboard";
let pendingCommands: Record<string, unknown>[] = [];

/*
Signal stores are mutated in place (ring.add never replaces the store's
state object), so selectors never fire. One counter per origin: after a batch
lands, the store's state is replaced with a fresh object carrying the bump and
the same ring map, which is what triggers re-renders. The counter lives on the
state itself so a worker reload cannot re-issue a stale identity.
*/
const bumpStore = (store: {
	state: Record<string, unknown>;
	setState: (s: never) => void;
}) => {
	const revision = Number(store.state.revision ?? 0) + 1;
	store.setState({ ...store.state, revision } as never);
};

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
							// Atom origins (balance, ...) carry their payload in
							// one layer; components read it via Value.lookup on
							// the atom's current Value.
							atoms[origin].set(val);
						}

						if (store) {
							const symbol = val.scope;
							let ring = store.state[symbol];

							if (!ring) {
								ring = new RingBuffer<Value>(50);
								store.state[symbol] = ring;
							}

							ring.add(val);
							bumpStore(store);
						}

						atoms.tickCount.set(Number(val.tick));
					}
				}
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
