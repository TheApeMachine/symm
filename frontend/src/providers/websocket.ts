import { useEffect } from "react";

import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { executionsStore } from "#/collections/executions";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { resonanceStore } from "#/collections/resonance";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const WIRE_ERROR_LOG_INTERVAL_MS = 5000;

let lastWireErrorAt = 0;

const routes = {
	measurement: measurementsStore.actions.updateReading,
	resonance: resonanceStore.actions.updateFrame,
	balances: balancesStore.actions.updateFrame,
	executions: executionsStore.actions.updateFrame,
	orders: ordersStore.actions.updateFrame,
} as const;

const wireBufferFromMessage = async (
	data: MessageEvent["data"],
): Promise<ArrayBuffer | null> => {
	if (data instanceof ArrayBuffer) {
		return data;
	}

	if (data instanceof Blob) {
		return data.arrayBuffer();
	}

	return null;
};

export const WsFeed = () => {
	const { updateOnline } = appStore.actions;

	useEffect(() => {
		const socket = new WebSocket(socketUrl);
		socket.binaryType = "arraybuffer";

		socket.addEventListener("open", () => {
			updateOnline(true);
		});

		socket.addEventListener("close", () => {
			updateOnline(false);
		});

		socket.addEventListener("message", (event) => {
			void (async () => {
				try {
					const buffer = await wireBufferFromMessage(event.data);

					if (buffer === null) {
						return;
					}

					const frame = await decodePackedArtifactWire(buffer);

					if (frame === null) {
						return;
					}

					const route = routes[frame.role as keyof typeof routes];

					route?.(frame);
				} catch (error) {
					const now = Date.now();

					if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
						return;
					}

					lastWireErrorAt = now;
					console.error("websocket frame parse failed", error);
				}
			})();
		});

		return () => {
			socket.close();
		};
	}, [updateOnline]);

	return null;
};
