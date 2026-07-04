import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { playbookStore } from "#/collections/playbook";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { tickStore } from "#/collections/tick";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

const routes: Record<string, (frame: Record<string, unknown>) => void> = {
	tick: tickStore.actions.updateFrame,
	instrument: instrumentsStore.actions.updateFrame,
	manifold: manifoldStore.actions.updateFrame,
	measurement: measurementsStore.actions.updateFrame,
	regime: measurementsStore.actions.updateFrame,
	resonance: resonanceStore.actions.updateFrame,
	balances: balancesStore.actions.updateFrame,
	executions: executionsStore.actions.updateFrame,
	fill: executionsStore.actions.updateFrame,
	quote: executionsStore.actions.updateFrame,
	orders: ordersStore.actions.updateFrame,
	order: ordersStore.actions.updateFrame,
	stoploss: ordersStore.actions.updateFrame,
	decision: decisionStore.actions.updateFrame,
	diagnostic: diagnosticsStore.actions.updateFrame,
	positions: positionsStore.actions.updateFrame,
	walk: playbookStore.actions.updateFrame,
	cognitive: cognitiveStore.actions.updateFrame,
};

export const routeFrame = (frame: Record<string, unknown>) => {
	const route = routes[String(frame.role)];

	if (route === undefined) {
		appStore.actions.updateError({
			err: "unrouted artifact role",
			role: frame.role,
			scope: frame.scope,
			origin: frame.origin,
		});
		return;
	}

	route(frame);
};

export const WsFeed = () => {
	const { updateOnline, updateError } = appStore.actions;

	useEffect(() => {
		let closedByUnmount = false;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let attempt = 0;
		let socket: WebSocket | null = null;
		let decodeWorker: Worker | null = null;

		const ensureDecodeWorker = (): Worker | null => {
			if (decodeWorker !== null) {
				return decodeWorker;
			}

			try {
				decodeWorker = new Worker(
					new URL("./websocket-decode.worker.ts", import.meta.url),
					{ type: "module" },
				);
				decodeWorker.addEventListener(
					"message",
					(
						event: MessageEvent<{
							frame?: Record<string, unknown>;
							error?: string;
						}>,
					) => {
						const frame = event.data.frame;
						const err = event.data.error;

						if (!frame || err) {
							console.error(err);
							updateError({ err: err });
							return;
						}

						if (frame) {
							routeFrame(frame);
						}
					},
				);
				decodeWorker.addEventListener("error", (event) => {
					console.error(event);
				});
			} catch (err) {
				console.error(err);
				updateError({ err: err });
				decodeWorker = null;
			}

			return decodeWorker;
		};

		const decodeAndQueue = (buffer: ArrayBuffer) => {
			const worker = ensureDecodeWorker();

			if (worker !== null) {
				worker.postMessage({ buffer }, [buffer]);
			}
		};

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
			socket = new WebSocket(socketUrl);
			socket.binaryType = "arraybuffer";

			socket.addEventListener("open", () => {
				if (closedByUnmount) {
					socket?.close();
					return;
				}

				attempt = 0;
				updateOnline(true);
			});

			socket.addEventListener("close", () => {
				if (closedByUnmount) {
					return;
				}

				updateOnline(false);
				scheduleReconnect();
			});

			socket.addEventListener("error", () => {
				if (socket?.readyState === WebSocket.OPEN) {
					socket.close();
				}
			});

			socket.addEventListener("message", (event) => {
				try {
					decodeAndQueue(event.data);
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

			decodeWorker?.terminate();

			if (socket?.readyState === WebSocket.OPEN) {
				socket.close();
			}
		};
	}, [updateOnline, updateError]);

	return null;
};
