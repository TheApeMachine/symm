/// <reference lib="webworker" />

let socket: WebSocket | null = null;

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
	if (
		socket !== null &&
		(socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING)
	) {
		socket.close();
	}

	socket = null;
};

const connect = (url: string) => {
	teardownSocket();

	socket = new WebSocket(url);
	socket.binaryType = "arraybuffer";

	socket.addEventListener("open", () => {
		self.postMessage({ type: "STATUS", status: "ONLINE" });
	});

	socket.addEventListener("close", () => {
		self.postMessage({ type: "STATUS", status: "OFFLINE" });
	});

	socket.addEventListener("error", (event) => {
		self.postMessage({ type: "ERROR", error: String(event) });
	});

	socket.addEventListener("message", (event) => {
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
