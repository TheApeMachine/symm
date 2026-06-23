type MessageListener = (event: MessageEvent) => void;
type ConnectionListener = (connected: boolean) => void;

const IDLE_DISCONNECT_MS = 150;

type SymmWsClient = {
	socket: WebSocket | null;
	socketUrl: string;
	messageListeners: Set<MessageListener>;
	connectionListeners: Set<ConnectionListener>;
	idleDisconnectTimer: ReturnType<typeof setTimeout> | null;
};

const symmWsClient: SymmWsClient = {
	socket: null,
	socketUrl: "",
	messageListeners: new Set(),
	connectionListeners: new Set(),
	idleDisconnectTimer: null,
};

const notifyConnection = (connected: boolean) => {
	for (const listener of symmWsClient.connectionListeners) {
		listener(connected);
	}
};

const closeSocket = () => {
	const socket = symmWsClient.socket;

	if (socket === null) {
		return;
	}

	symmWsClient.socket = null;

	if (
		socket.readyState === WebSocket.OPEN ||
		socket.readyState === WebSocket.CONNECTING
	) {
		socket.close();
	}
};

const scheduleIdleDisconnect = () => {
	if (symmWsClient.idleDisconnectTimer !== null) {
		clearTimeout(symmWsClient.idleDisconnectTimer);
	}

	symmWsClient.idleDisconnectTimer = setTimeout(() => {
		symmWsClient.idleDisconnectTimer = null;

		if (symmWsClient.messageListeners.size === 0) {
			closeSocket();
		}
	}, IDLE_DISCONNECT_MS);
};

const ensureSocket = (socketUrl: string) => {
	if (symmWsClient.idleDisconnectTimer !== null) {
		clearTimeout(symmWsClient.idleDisconnectTimer);
		symmWsClient.idleDisconnectTimer = null;
	}

	if (
		symmWsClient.socket !== null &&
		symmWsClient.socketUrl === socketUrl &&
		(symmWsClient.socket.readyState === WebSocket.OPEN ||
			symmWsClient.socket.readyState === WebSocket.CONNECTING)
	) {
		return;
	}

	closeSocket();

	symmWsClient.socketUrl = socketUrl;
	symmWsClient.socket = new WebSocket(socketUrl);
	symmWsClient.socket.binaryType = "arraybuffer";

	symmWsClient.socket.addEventListener("open", () => {
		notifyConnection(true);
	});

	symmWsClient.socket.addEventListener("close", () => {
		if (symmWsClient.socket !== null) {
			symmWsClient.socket = null;
		}

		notifyConnection(false);

		if (symmWsClient.messageListeners.size > 0) {
			ensureSocket(socketUrl);
		}
	});

	symmWsClient.socket.addEventListener("message", (event) => {
		for (const listener of symmWsClient.messageListeners) {
			listener(event);
		}
	});
};

/*
subscribeSymmWsFeed attaches a dashboard listener to the shared symm websocket.
Strict-mode remounts reuse the same socket instead of opening parallel connections.
*/
export const subscribeSymmWsFeed = (
	socketUrl: string,
	onMessage: MessageListener,
	onConnection: ConnectionListener,
) => {
	symmWsClient.messageListeners.add(onMessage);
	symmWsClient.connectionListeners.add(onConnection);

	ensureSocket(socketUrl);

	onConnection(symmWsClient.socket?.readyState === WebSocket.OPEN);

	return () => {
		symmWsClient.messageListeners.delete(onMessage);
		symmWsClient.connectionListeners.delete(onConnection);

		if (symmWsClient.messageListeners.size === 0) {
			scheduleIdleDisconnect();
		}
	};
};
