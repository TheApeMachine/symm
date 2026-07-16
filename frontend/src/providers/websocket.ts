import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { applyFramePayload } from "#/providers/ws-stores";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DATA_UPDATE"; payload: Record<string, unknown> }
	| { type: "ERROR"; message: string };

let worker: Worker | null = null;

const disconnectTransport = () => {
	if (worker !== null) {
		worker.postMessage({ type: "DISCONNECT" });
		worker.terminate();
		worker = null;
	}
};

const handleWorkerMessage = (event: MessageEvent<WorkerOutbound>) => {
	const message = event.data;

	if (message.type === "READY") {
		worker?.postMessage({ type: "CONNECT", url: socketUrl });
		return;
	}

	if (message.type === "ONLINE") {
		appStore.actions.updateOnline(message.online);
		return;
	}

	if (message.type === "DATA_UPDATE") {
		applyFramePayload(message.payload);
		return;
	}

	if (message.type === "ERROR") {
		appStore.actions.updateError({ message: message.message });
	}
};

const connect = () => {
	disconnectTransport();

	worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
		type: "module",
	});

	worker.addEventListener("message", handleWorkerMessage);
	worker.addEventListener("error", (event) => {
		console.error("WS worker failed:", event.message);
		appStore.actions.updateOnline(false);
		appStore.actions.updateError({ message: event.message });
	});
};

/*
WsFeed boots the websocket worker once. The worker owns the socket, decodes
every backend frame, and pushes coalesced payloads into the TanStack stores in
16ms increments. This is the only transport path.
*/
export const WsFeed = () => {
	useEffect(() => {
		connect();

		return () => {
			disconnectTransport();
		};
	}, []);

	return null;
};

export { applyFramePayload };
