import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	closedPositionsAtom,
	decisionsAtom,
	errorAtom,
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
import { TickFrame } from "#/providers/telemetry/telemetry/tick-frame";
import { topologyStore } from "#/collections/topology";

let globalWsWorkers: Worker[] = [];

export const getWsWorker = () => globalWsWorkers[0] ?? null;

export const sendPositionExit = (symbol: string) => {
	for (const worker of globalWsWorkers) {
		worker.postMessage({
			type: "POSITION_EXIT",
			symbol,
		});
	}
};

export const sendRoute = (route: string) => {
	for (const worker of globalWsWorkers) {
		worker.postMessage({
			type: "ROUTE",
			route,
		});
	}
};

const defaultWsUrls = (): string[] => {
	if (typeof window === "undefined") {
		return ["ws://127.0.0.1:8765/ws/0", "ws://127.0.0.1:8765/ws/1"];
	}
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host = window.location.hostname || "127.0.0.1";
	return [
		`${protocol}//${host}:8765/ws/0`,
		`${protocol}//${host}:8765/ws/1`,
	];
};

/*
Pending display state accumulated between animation frames. Ring buffers are
mutated as frames arrive; subscribers are notified once per display frame so a
burst of websocket messages costs one render, not one render per message.
*/
const pendingSources = new Set<string>();
let pendingTick: bigint | null = null;
let pendingAt: bigint | null = null;
let flushHandle: number | null = null;

const flush = () => {
	flushHandle = null;

	storeBatch(() => {
		if (pendingTick !== null) {
			tickCountAtom.set(Number(pendingTick));
			pendingTick = null;
		}

		if (pendingAt !== null) {
			updateClock(pendingAt);
			pendingAt = null;
		}

		for (const source of pendingSources) {
			signals[source]?.setState((prev) => ({ ...prev }));
		}

		pendingSources.clear();
	});
};

const scheduleFlush = () => {
	if (flushHandle !== null) {
		return;
	}

	if (typeof requestAnimationFrame !== "undefined") {
		flushHandle = requestAnimationFrame(flush);
	} else {
		flushHandle = setTimeout(flush, 16) as unknown as number;
	}
};

/*
Dispatches one decoded MeasurementsFrame into per-measurement rings.
Each Measurement row carries its own source, symbol, tick, snr, and metrics.
Engine tick and clock are owned by TickFrame, never by individual rows.
*/
export function dispatchMeasurements(frame: MeasurementsFrame) {
	topologyStore.actions.ingestMeasurements(frame);

	const count = frame.rowsLength();

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
		pendingSources.add(source);
	}

	if (pendingSources.size > 0) {
		scheduleFlush();
	}
}

/*
dispatchTick records the hub's live ingress progress for the next display frame.
*/
const dispatchTick = (frame: TickFrame) => {
	pendingTick = frame.count();

	if (frame.at() > 0n) {
		pendingAt = frame.at();
	}

	scheduleFlush();
};

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
		const urls = import.meta.env.VITE_SYMM_WS_URL
			? [import.meta.env.VITE_SYMM_WS_URL]
			: defaultWsUrls();

		const workers = urls.map((url, shardIndex) => {
			const worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
				type: "module",
			});

			worker.addEventListener("message", (event: MessageEvent) => {
				const data = event.data;
				if (!data) return;

				if (data.type === "STATUS") {
					onlineAtom.set(data.status);
					if (data.status === "ONLINE") {
						worker.postMessage({ type: "FOCUS", symbol: focusAtom.get() });
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
					console.error(`WS[${shardIndex}] error:`, data.error);
					errorAtom.set({
						error: String(data.error),
						source: `WebSocket Shard ${shardIndex}`,
						url,
					});
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
									dispatchMeasurements(measurementsFrame);
								}
								return;
							}

							if (frameType === Frame.TickFrame) {
								const tickFrame = message.frame(new TickFrame());
								if (tickFrame) {
									dispatchTick(tickFrame);
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
									const unpacked = positionsFrame.unpack();
									positionsAtom.set(unpacked.rows);
									closedPositionsAtom.set(unpacked.closed);
									positionCountAtom.set(unpacked.rows.length);
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
						}

						const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);
						dispatchMeasurements(frame);
					} catch (err) {
						console.error(`WS[${shardIndex}] message processing error:`, err);
					}
				}
			});

			worker.postMessage({ type: "ROUTE", route: routeAtom.get() });
			worker.postMessage({ type: "CONNECT", url });
			return worker;
		});

		globalWsWorkers = workers;

		const unsubscribeFocus = focusAtom.subscribe((symbol: string) => {
			for (const worker of workers) {
				worker.postMessage({ type: "FOCUS", symbol });
			}
		});

		const unsubscribeRoute = routeAtom.subscribe((route: string) => {
			for (const worker of workers) {
				worker.postMessage({ type: "ROUTE", route });
			}
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			for (const worker of workers) {
				worker.postMessage({ type: "DISCONNECT" });
				worker.terminate();
			}
			globalWsWorkers = [];

			if (flushHandle !== null) {
				cancelAnimationFrame(flushHandle);
				flushHandle = null;
			}
		};
	}, []);

	return null;
};
