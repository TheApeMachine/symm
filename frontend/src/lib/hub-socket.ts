type HubStatus = "ONLINE" | "OFFLINE" | "CONNECTING";

type HubHandlers = {
	onEnvelope: (payload: Record<string, unknown>) => void;
	onStatus: (status: HubStatus) => void;
	onError?: (error: string) => void;
};

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;
let handlers: HubHandlers | null = null;

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

	pendingCommands = [];
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
	if (reconnectTimer !== null || handlers === null) {
		return;
	}

	const delay = Math.min(
		RECONNECT_BASE_MS * 2 ** reconnectAttempts,
		RECONNECT_MAX_MS,
	);

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		reconnectAttempts += 1;
		openSocket(connectUrl);
	}, delay);
};

const openSocket = (url: string) => {
	if (url === "" || handlers === null) {
		return;
	}

	teardownSocket();

	connectUrl = url;
	const generation = socketGeneration;
	handlers.onStatus("CONNECTING");
	socket = new WebSocket(url);

	socket.addEventListener("open", () => {
		if (generation !== socketGeneration) {
			return;
		}

		reconnectAttempts = 0;
		handlers?.onStatus("ONLINE");
		if (currentRoute) {
			sendRoute(currentRoute);
		}
		flushCommands();
	});

	socket.addEventListener("close", () => {
		if (generation !== socketGeneration) {
			return;
		}

		handlers?.onStatus("OFFLINE");
		scheduleReconnect();
	});

	socket.addEventListener("error", (event) => {
		if (generation !== socketGeneration) {
			return;
		}

		handlers?.onError?.(String(event));
	});

	socket.addEventListener("message", (event) => {
		if (typeof event.data !== "string" || event.data.length === 0) {
			return;
		}

		try {
			handlers?.onEnvelope(
				JSON.parse(event.data) as Record<string, unknown>,
			);
		} catch (err) {
			console.error("WS envelope parse failed:", err);
			handlers?.onError?.(String(err));
		}
	});
};

export const connectHub = (url: string, next: HubHandlers) => {
	handlers = next;
	openSocket(url);
};

export const disconnectHub = () => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	reconnectAttempts = 0;
	pendingCommands = [];
	handlers = null;
	teardownSocket();
};

export const sendRoute = (route: string) => {
	currentRoute = route;
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof route === "string"
	) {
		socket.send(JSON.stringify({ type: "route", route }));
	}
};

export const sendFocus = (symbol: string) => {
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof symbol === "string"
	) {
		socket.send(JSON.stringify({ type: "focus", symbol }));
	}
};

export const sendCommand = (command: Record<string, unknown>) => {
	if (handlers === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}

	if (!command) {
		throw new Error("hub command is empty");
	}

	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify(command));
		return;
	}

	queueCommand(command);
};

/** Best-effort hub command; false when the socket is not up. */
export const trySendCommand = (command: Record<string, unknown>): boolean => {
	try {
		sendCommand(command);
		return true;
	} catch {
		return false;
	}
};
