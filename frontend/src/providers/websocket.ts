import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { attach } from "#/providers/ws-stores";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "ERROR"; message: string }
	| { type: "ERROR_FRAME"; frame: Record<string, unknown> };

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
		if (worker !== null) {
			worker.postMessage({ type: "CONNECT", url: socketUrl });
			worker.postMessage({
				type: "FOCUS",
				symbol: appStore.state.focusSymbol,
			});
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

	attach(worker);
	worker.addEventListener("message", handleWorkerMessage);
	worker.addEventListener("error", (event) => {
		console.error("WS worker failed:", event.message);
		appStore.actions.updateOnline(false);
		appStore.actions.updateError({ message: event.message });
	});
};

/*
WsFeed boots the websocket worker once. The worker owns the socket and forwards
DRAW frames; attach dispatches wire keys to paint functions.
*/
export const WsFeed = () => {
	useEffect(() => {
		connect();
		const subscription = appStore.subscribe(() => {
			worker?.postMessage({
				type: "FOCUS",
				symbol: appStore.state.focusSymbol,
			});
		});

		return () => {
			subscription.unsubscribe();
			disconnectTransport();
		};
	}, []);

	return null;
};
