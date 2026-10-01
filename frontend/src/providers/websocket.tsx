import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	decisionsAtom,
	focusAtom,
	observeSymbols,
	onlineAtom,
	positionCountAtom,
	positionsAtom,
	RingBuffer,
	routeAtom,
	signals,
	symbolsAtom,
	tickCountAtom,
	updateClock,
	updateEquity,
} from "#/collections/app";

import { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { Message } from "#/providers/telemetry/telemetry/message";
import { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import { StrategyFrame } from "#/providers/telemetry/telemetry/strategy-frame";

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


type ManifoldListener = (bytes: Uint8Array) => void;
const manifoldListeners = new Set<ManifoldListener>();

export const subscribeManifold = (listener: ManifoldListener) => {
	manifoldListeners.add(listener);
	return () => {
		manifoldListeners.delete(listener);
	};
};

const publishManifold = (bytes: Uint8Array) => {
	for (const listener of manifoldListeners) {
		listener(bytes);
	}
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
export function dispatchMeasurements(frame: MeasurementsFrame) {
	const count = frame.rowsLength();
	const touched = new Set<string>();

	for (let i = 0; i < count; i++) {
		const row = frame.rows(i);
		if (!row) continue;

		const rawSource = (row.source() ?? "").toLowerCase();
		const source = rawSource.includes(":")
			? rawSource.split(":")[0]
			: rawSource;
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

		const signalStore = signals[source];

		if (!signalStore) {
			continue;
		}

		let ring = signalStore.state[symbol];

		if (!ring) {
			// Training historical replay can burst thousands of frames per
			// excursion; a 50-slot ring drops the mid-fragment path and leaves
			// the Model Training tape blank / incomplete.
			const capacity = source === "training" ? 4096 : 50;
			ring = new RingBuffer<MeasurementT>(capacity);
			signalStore.state[symbol] = ring;
		}

		ring.add(row.unpack());
		touched.add(source);
	}

	for (const source of touched) {
		signals[source]?.setState((prev) => ({ ...prev }));
	}
}


/*
dispatchMeasurementsBuffer accepts a FlatBuffer ByteBuffer that is either a
SYMM Message wrapping MeasurementsFrame or a bare MeasurementsFrame root.
*/
export const dispatchMeasurementsBuffer = (buffer: flatbuffers.ByteBuffer) => {
	if (Message.bufferHasIdentifier(buffer)) {
		const message = Message.getRootAsMessage(buffer);
		const frame = message.frame(new MeasurementsFrame());
		if (frame) {
			dispatchMeasurements(frame);
		}
		return;
	}

	const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);
	dispatchMeasurements(frame);
};

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
				for (const source of Object.keys(signals)) {
					delete signals[source]?.state[data.symbol];
				}
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

					if (Message.bufferHasIdentifier(buffer)) {
						const message = Message.getRootAsMessage(buffer);
						const frameType = message.frameType();

						if (frameType === Frame.MeasurementsFrame) {
							const measurementsFrame = message.frame(new MeasurementsFrame());
							if (measurementsFrame) {
								storeBatch(() => {
									dispatchMeasurements(measurementsFrame);
								});
							}
							return;
						}

						if (frameType === Frame.EquityFrame) {
							const equityFrame = message.frame(new EquityFrame());
							if (equityFrame) {
								updateEquity(
									equityFrame.cash(),
									equityFrame.unrealized(),
									equityFrame.equity(),
								);
							}
							return;
						}

						if (frameType === Frame.PositionsFrame) {
							const positionsFrame = message.frame(new PositionsFrame());
							if (positionsFrame) {
								const rows = positionsFrame.unpack().rows;
								positionsAtom.set(rows);
								positionCountAtom.set(rows.length);
							}
							return;
						}

						if (frameType === Frame.StrategyFrame) {
							const strategyFrame = message.frame(new StrategyFrame());
							if (strategyFrame) {
								const decisions = strategyFrame.unpack().decisions;
								decisionsAtom.set(decisions);
							}
							return;
						}

						if (frameType === Frame.ManifoldFrame) {
							publishManifold(bytes);
							return;
						}
					}

					const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);

					storeBatch(() => {
						dispatchMeasurements(frame);
					});
				} catch (err) {
					console.error("WS message processing error:", err);
				}
			}
		});

		wsWorker.postMessage({ type: "ROUTE", route: routeAtom.get() });
		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });

		const unsubscribeFocus = focusAtom.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = routeAtom.subscribe((route: string) => {
			wsWorker.postMessage({ type: "ROUTE", route });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			globalWsWorker = null;
		};
	}, []);

	return null;
};
