import { batch } from "@tanstack/store";
import { useEffect } from "react";
import { actionStore } from "#/collections/actions";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { causalStore } from "#/collections/causal";
import { cognitiveStore } from "#/collections/cognitive";
import { executionsStore } from "#/collections/executions";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { stopsStore } from "#/collections/stops";
import { tickStore } from "#/collections/tick";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

type FrameStore = {
	actions: {
		updateFrame: (frame: unknown) => void;
	};
};

const stores = {
	actions: actionStore,
	balance: balancesStore,
	balances: balancesStore,
	causal: causalStore,
	cognitive: cognitiveStore,
	intents: actionStore,
	positions: positionsStore,
	stops: stopsStore,
	executions: executionsStore,
	instruments: instrumentsStore,
	measurements: measurementsStore,
	manifold: manifoldStore,
	orders: ordersStore,
	resonance: resonanceStore,
	tick: tickStore,
} as Record<string, FrameStore>;

let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let initialized = false;

const scheduleReconnect = () => {
	if (reconnectTimer !== null) return;

	const delay = Math.min(RECONNECT_MAX_MS, RECONNECT_BASE_MS * 2 ** attempt);
	attempt += 1;

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		connect();
	}, delay);
};

const connect = () => {
	// Terminate any stale sockets before spinning up a new one
	if (socket) {
		socket.onopen = null;
		socket.onclose = null;
		socket.onerror = null;
		socket.onmessage = null;
		if (
			socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING
		) {
			socket.close();
		}
	}

	const currentSocket = new WebSocket(socketUrl);
	socket = currentSocket;

	currentSocket.addEventListener("open", () => {
		if (socket !== currentSocket) return;
		attempt = 0;
		appStore.actions.updateOnline(true);
	});

	currentSocket.addEventListener("close", () => {
		if (socket !== currentSocket) return;
		appStore.actions.updateOnline(false);
		scheduleReconnect();
	});

	currentSocket.addEventListener("error", () => {
		if (socket !== currentSocket) return;
		if (currentSocket.readyState === WebSocket.OPEN) {
			currentSocket.close();
		}
	});

	currentSocket.addEventListener("message", (event) => {
		if (socket !== currentSocket) return;

		try {
			const parsedData = JSON.parse(String(event.data));

			batch(() => {
				for (const [key, data] of Object.entries(parsedData)) {
					// Guard clause to protect against undefined store targets
					if (stores[key]?.actions) {
						stores[key].actions.updateFrame(data);
					} else {
						console.warn(`No store found matching frame key: "${key}"`);
					}
				}
			});
		} catch (err) {
			console.error("WS Parse Error:", err);
			appStore.actions.updateError({ err });
		}
	});
};

export const WsFeed = () => {
	useEffect(() => {
		if (!initialized) {
			initialized = true;
			connect();
		}
	}, []);

	return null;
};
