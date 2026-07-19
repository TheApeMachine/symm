import { useEffect } from "react";
import { appStore } from "#/collections/app";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "ERROR"; message: string }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> };

	
	
let worker: Worker | null = null;	
export const getWorker = () => worker;

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
		if (worker !== null) {
			worker.postMessage({ type: "CONNECT", url: socketUrl });
		}

		return;
	}

	if (message.type === "ONLINE") {
		appStore.actions.updateOnline(message.online);
		return;
	}

	if (message.type === "ERROR_FRAME") {
		appStore.actions.updateError(message.frame);
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
frames, and retains desk state. Main-thread mirrors subscribe over MessageChannel;
app/terminal stay on the UI thread.
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
