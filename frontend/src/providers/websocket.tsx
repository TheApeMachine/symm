import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	evictStaleSymbols,
	evictSymbol,
	focusAtom,
	manifoldStore,
	observeSymbols,
	onlineAtom,
	RingBuffer,
	routeAtom,
	signals,
	symbolsAtom,
	tickCountAtom,
	updateClock,
} from "#/collections/app";

import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";

let globalWsWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;

export const sendPositionExit = (symbol: string) => {
	globalWsWorker?.postMessage({
		type: "POSITION_EXIT",
		symbol,
	});
};

export const sendRoute = (route: string) => {
	globalWsWorker?.postMessage({
		type: "ROUTE",
		route,
	});
};

const defaultWsUrl = () => {
	if (typeof window === "undefined") return "ws://127.0.0.1:8765/ws";
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host = window.location.hostname || "127.0.0.1";
	return `${protocol}//${host}:8765/ws`;
};

/*
Dispatches one decoded MeasurementsFrame into per-measurement rings.
Each Measurement row carries its own source, symbol, tick, snr, and metrics.
*/
function dispatchMeasurements(frame: MeasurementsFrame) {
	const count = frame.rowsLength();
	const touched = new Set<string>();

	for (let i = 0; i < count; i++) {
		const row = frame.rows(i);
		if (!row) continue;

		const rawSource = (row.source() ?? "").toLowerCase();
		const source = rawSource.includes(":") ? rawSource.split(":")[0] : rawSource;
		const symbol = row.symbol() ?? "";

		if (symbol && !symbolsAtom.get().includes(symbol)) {
			observeSymbols([symbol]);
		}

		const at = row.at();
		if (at > 0n) {
			updateClock(at);
		}
		const tick = row.tick();
		if (tick > 0n) {
			tickCountAtom.set(Number(tick));
		}

		if (source === "manifold") {
			manifoldStore.setState((prev: Record<string, unknown>) => ({
				...prev,
				[symbol]: row.unpack(),
			}));
			touched.add(source);
			continue;
		}

		const signalStore = signals[source];
		if (!signalStore) {
			continue;
		}

		let ring = signalStore.state[symbol];

		if (!ring) {
			ring = new RingBuffer<MeasurementT>(50);
			signalStore.state[symbol] = ring;
		}

		ring.add(row.unpack());
		touched.add(source);

		if (source === "training") {
			signalStore.state[""] = ring;
			signalStore.state["learner"] = ring;
			const currentFocus = focusAtom.get();

			if (currentFocus) {
				signalStore.state[currentFocus] = ring;
			}
		}
	}

	for (const source of touched) {
		signals[source]?.setState((prev) => ({ ...prev }));
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
				onlineAtom.set(data.status);
				if (data.status === "ONLINE") {
					wsWorker.postMessage({ type: "FOCUS", symbol: focusAtom.get() });
				}
				return;
			}

			if (data.type === "UNSUBSCRIBE" && typeof data.symbol === "string") {
				evictSymbol(data.symbol);
				return;
			}

			if (data.type === "ERROR") {
				console.error("WS error:", data.error);
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
					const bytes = new Uint8Array(data.buffer);
					const buffer = new flatbuffers.ByteBuffer(bytes);

					const frame =
						MeasurementsFrame.getRootAsMeasurementsFrame(buffer);

					storeBatch(() => {
						dispatchMeasurements(frame);
					});
				} catch (err) {
					console.error("WS message processing error:", err);
				}
			}
		});

		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });

		const unsubscribeFocus = focusAtom.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = routeAtom.subscribe((route: string) => {
			wsWorker.postMessage({ type: "ROUTE", route });
		});

		const evictionInterval = setInterval(() => {
			evictStaleSymbols();
		}, 60_000);

		return () => {
			clearInterval(evictionInterval);
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			globalWsWorker = null;
		};
	}, []);

	return null;
};
