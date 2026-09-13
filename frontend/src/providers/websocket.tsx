import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	addMeasurement,
	appStore,
	errorStore,
	focusStore,
	onlineStore,
	tickCountStore,
} from "#/collections/app";

import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";

let globalWsWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;

export const sendPositionExit = (symbol: string) => {
	globalWsWorker?.postMessage({
		type: "POSITION_EXIT",
		symbol,
	});
};

const defaultWsUrl = () => {
	if (typeof window === "undefined") return "ws://127.0.0.1:8765/ws";
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;
	return `${protocol}//${host}:8765/ws`;
};

/*
Dispatches one decoded MeasurementsFrame into per-measurement rings.
Each Measurement row carries its own source, symbol, tick, snr, and metrics.
*/
function dispatchMeasurements(frame: MeasurementsFrame) {
	const count = frame.rowsLength();

	for (let i = 0; i < count; i++) {
		const row = frame.rows(i);
		if (!row) continue;

		const source = row.source() ?? "";
		addMeasurement(source, row);

		const tick = row.tick();
		if (tick > 0n) {
			tickCountStore.setState(() => Number(tick));
		}
	}
}

export const WsFeed = () => {
	useEffect(() => {
		const wsWorker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});
		globalWsWorker = wsWorker;

		const wsUrl = import.meta.env.VITE_SYMM_WS_URL || defaultWsUrl();

		wsWorker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "STATUS") {
				onlineStore.setState(() => data.status);
				appStore.actions.updateOnline(data.status === "ONLINE");
				if (data.status === "ONLINE") {
					wsWorker.postMessage({ type: "FOCUS", symbol: focusStore.state });
				}
				return;
			}

			if (data.type === "ERROR") {
				errorStore.setState(() => new Error(data.error));
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
					const buffer = new flatbuffers.ByteBuffer(
						new Uint8Array(data.buffer),
					);

					const frame =
						MeasurementsFrame.getRootAsMeasurementsFrame(buffer);

					storeBatch(() => {
						dispatchMeasurements(frame);
					});
				} catch (err) {
					errorStore.setState(() => err as Event);
				}
			}
		});

		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });

		const unsubscribeFocus = focusStore.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			globalWsWorker = null;
		};
	}, []);

	return null;
};
