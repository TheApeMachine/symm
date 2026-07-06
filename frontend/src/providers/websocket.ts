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
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
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
	positions: positionsStore,
	executions: executionsStore,
	instruments: instrumentsStore,
	measurements: measurementsStore,
	manifold: manifoldStore,
	resonance: resonanceStore,
	tick: tickStore,
} as Record<string, FrameStore>;

export const WsFeed = () => {
	const { updateOnline, updateError } = appStore.actions;

	useEffect(() => {
		let closedByUnmount = false;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let attempt = 0;
		let socket: WebSocket | null = null;

		const scheduleReconnect = () => {
			if (closedByUnmount || reconnectTimer !== null) {
				return;
			}

			const delay = Math.min(
				RECONNECT_MAX_MS,
				RECONNECT_BASE_MS * 2 ** attempt,
			);

			attempt += 1;
			reconnectTimer = setTimeout(() => {
				reconnectTimer = null;
				connect();
			}, delay);
		};

		const connect = () => {
			const currentSocket = new WebSocket(socketUrl);
			socket = currentSocket;

			currentSocket.addEventListener("open", () => {
				if (closedByUnmount || socket !== currentSocket) {
					currentSocket.close();
					return;
				}

				attempt = 0;
				updateOnline(true);
			});

			currentSocket.addEventListener("close", () => {
				if (closedByUnmount || socket !== currentSocket) {
					return;
				}

				updateOnline(false);
				scheduleReconnect();
			});

			currentSocket.addEventListener("error", () => {
				if (socket !== currentSocket) {
					return;
				}
				if (currentSocket.readyState === WebSocket.OPEN) {
					currentSocket.close();
				}
			});

			currentSocket.addEventListener("message", (event) => {
				if (socket !== currentSocket) {
					return;
				}

				try {
					batch(() => {
						for (const [key, data] of Object.entries(
							JSON.parse(String(event.data)),
						)) {
							stores[key].actions.updateFrame(data);
						}
					});
				} catch (err) {
					console.error(err);
					updateError({ err: err });
				}
			});
		};

		connect();

		return () => {
			closedByUnmount = true;

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
			}

			if (socket?.readyState === WebSocket.OPEN) {
				socket.close();
			}
		};
	}, [updateOnline, updateError]);

	return null;
};
