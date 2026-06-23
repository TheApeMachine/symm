import { useEffect } from "react";

import { appStore } from "#/collections/app";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";
import { subscribeSymmWsFeed } from "#/providers/symm-ws-client";
import { routeDecodedFrame } from "#/providers/websocket-handlers";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const WIRE_ERROR_LOG_INTERVAL_MS = 5000;

let lastWireErrorAt = 0;

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
		return subscribeSymmWsFeed(
			socketUrl,
			(event) => {
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

						console.log("frame", frame);

						routeDecodedFrame(frame);
					} catch (error) {
						const now = Date.now();

						if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
							return;
						}

						lastWireErrorAt = now;
						console.error("websocket frame parse failed", error);
					}
				})();
			},
			(connected) => updateOnline(connected),
		);
	}, [updateOnline]);

	return null;
};
