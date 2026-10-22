import { atoms } from "#/collections/app";
import { useEffect } from "react";

/*
The hub multiplexes everything over one socket: each message is one serialized
*data.Value, routed here by its layer index. Commands go back on the same socket.
*/
let wsWorker: Worker | null = null;
const hubWsUrl = () => {
	const url = new URL(window.location.href);
	url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
	url.pathname = "/ws";
	return url.toString();
}

const toBackendRoute = (route: string) => {
	// The learning surface hosts the Impulse Map and Position Health inspector,
	// which require focused signal streams (CVD, Hawkes, Depthflow, etc.) to light up
	// the SOM grid confluence regions.
	return route === "learning" ? "dashboard" : route;
};

export const sendRoute = (route: string) => {
	wsWorker?.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
};

/*
sendCommand sends one JSON command the hub answers on this socket. With no
socket the command cannot be sent, which is an error, not a no-op.
*/
export const sendCommand = (command: Record<string, unknown>) => {
	if (wsWorker === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}

	wsWorker.postMessage({ type: "COMMAND", command });
};

export const WsFeed = () => {
	useEffect(() => {
		const url = `${hubWsUrl()}/ws`;
		const worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});

		worker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "COMMAND_FAILED") {
				console.error("hub command not sent:", data.command, data.error);
				return;
			}

			if (data.type === "STATUS") {
				return;
			}

			if (data.type === "ERROR") {
				console.error("WS error:", data.error);
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
				} catch (err) {
					console.error("WS message processing error:", err);
				}
			}
		});

		worker.postMessage({
			type: "ROUTE",
			route: toBackendRoute(atoms.route.get()),
		});
		worker.postMessage({ type: "CONNECT", url });
		wsWorker = worker;

		const unsubscribeFocus = atoms.focus.subscribe((symbol: string) => {
			worker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = atoms.route.subscribe((route: string) => {
			worker.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			wsWorker = null;
		};
	}, []);

	return null;
};
