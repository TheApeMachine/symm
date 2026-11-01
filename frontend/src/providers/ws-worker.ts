/// <reference lib="webworker" />

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

let currentRoute = "dashboard";
let pendingCommands: Record<string, unknown>[] = [];

const textDecoder = new TextDecoder();

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

const asArrayBuffer = (raw: unknown): ArrayBuffer | null => {
	if (raw instanceof ArrayBuffer) {
		return raw;
	}

	if (raw instanceof Uint8Array) {
		return raw.buffer.slice(
			raw.byteOffset,
			raw.byteOffset + raw.byteLength,
		) as ArrayBuffer;
	}

	if (
		raw &&
		typeof raw === "object" &&
		"buffer" in raw &&
		(raw as { buffer: unknown }).buffer instanceof ArrayBuffer
	) {
		const view = raw as {
			buffer: ArrayBuffer;
			byteOffset: number;
			byteLength: number;
		};
		return view.buffer.slice(
			view.byteOffset,
			view.byteOffset + view.byteLength,
		);
	}

	return null;
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
		try {
			const arrayBuffer = asArrayBuffer(event.data);
			if (!arrayBuffer || arrayBuffer.byteLength === 0) {
				return;
			}

			const bytes = new Uint8Array(arrayBuffer);
			// Hub dashboard traffic is JSON envelopes only.
			if (bytes[0] !== 0x7b /* '{' */) {
				return;
			}

			self.postMessage({
				type: "ENVELOPE",
				payload: JSON.parse(textDecoder.decode(bytes)),
			});
		} catch (err) {
			console.error("error", err);
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
