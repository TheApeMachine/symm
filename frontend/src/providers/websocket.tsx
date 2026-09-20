import { batch as storeBatch } from "@tanstack/react-store";
import { useEffect } from "react";
import { MessageReader } from "@naeemo/capnp";
import {
	evictStaleSymbols,
	evictSymbol,
	focusAtom,
	onlineAtom,
	RingBuffer,
	routeAtom,
	signals,
	symbolsAtom,
	tickCountAtom,
	updateClock,
	updateEquity,
} from "#/collections/app";

import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type { MetricT } from "#/providers/telemetry/telemetry/metric";

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

const SOURCE_TYPES = ['manifold', 'decision', 'strategy', 'equity', 'balance', 'stream'];

function decodeWireMeasurement(buffer: ArrayBuffer): MeasurementT {
	const reader = new MessageReader(buffer);
	const root = reader.getRoot(7, 4); 

	const idData = root.getData(0);
	const id = new TextDecoder().decode(idData);

	const symbolData = root.getData(1);
	const symbol = new TextDecoder().decode(symbolData);

	const tick = root.getInt64(8);
	const at = root.getInt64(0);
	const timestamp = root.getInt64(16);
	const snr = root.getFloat64(32);
	const maturity = root.getFloat64(40);
	const separation = root.getFloat64(48);

	const sourceIdx = root.getUint16(26);
	const source = SOURCE_TYPES[sourceIdx] || 'unknown';

	// Parse metrics
	const metrics: MetricT[] = [];
	const metricsList = root.getList(2);
	if (metricsList) {
		const mCount = metricsList.length;
		for (let i = 0; i < mCount; i++) {
			const mStruct = metricsList.getStruct(i);
			if (!mStruct) continue;
			metrics.push({
				raw: mStruct.getFloat64(0),
				normalized: mStruct.getFloat64(8),
				standardized: mStruct.getFloat64(16),
				center: mStruct.getFloat64(24),
				support: mStruct.getFloat64(32),
				variance: mStruct.getFloat64(40),
				snr: mStruct.getFloat64(48),
				hasNormalized: true,
			});
		}
	}

	// Parse metadata map
	const metadata: Record<string, string | number | boolean> = {};
	const metadataStruct = root.getStruct(3);
	if (metadataStruct) {
		const entriesList = metadataStruct.getList(0);
		if (entriesList) {
			const eCount = entriesList.length;
			for (let i = 0; i < eCount; i++) {
				const entryStruct = entriesList.getStruct(i);
				if (!entryStruct) continue;
				
				const keyData = entryStruct.getText(0);
				const key = keyData || "";

				const valueStruct = entryStruct.getStruct(1);
				if (valueStruct) {
					const tag = valueStruct.getUint16(0);
					if (tag === 1) { // text
						metadata[key] = valueStruct.getText(0) || "";
					} else if (tag === 2) { // int
						metadata[key] = Number(valueStruct.getInt64(8));
					} else if (tag === 3) { // float
						metadata[key] = valueStruct.getFloat64(8);
					} else if (tag === 4) { // bool
						metadata[key] = valueStruct.getUint8(8) !== 0;
					}
				}
			}
		}
	}

	return {
		id,
		source,
		symbol,
		tick,
		at,
		snr,
		maturity,
		separation,
		metrics,
		metadata: metadata as any,
	};
}

function dispatchMeasurement(row: MeasurementT) {
	const source = row.source;
	const symbol = row.symbol;

	const symbolStr = (row.symbol as string) || "";
	if (symbolStr && !symbolsAtom.get().includes(symbolStr)) {
		symbolsAtom.set(Array.from(new Set([...symbolsAtom.get(), symbolStr])));
	}

	if (row.at > 0n) {
		updateClock(row.at);
	}
	if (row.tick > 0n) {
		tickCountAtom.set(Number(row.tick));
	}

	if (source === "manifold") {
		signals.manifold.setState((prev: any) => ({
			...prev,
			[symbolStr]: row,
		}));
		signals.manifold.setState((prev: any) => ({ ...prev }));
		return;
	}

	if (source === "decision" || source === "strategy") {
		const meta = (row as any).metadata || {};
		let action = String(meta["action"] || "wait");
		let reason = String(meta["reason"] || "attractor transition basin");
		let confidence = meta["confidence"] !== undefined ? Number(meta["confidence"]) : row.snr;

		signals.strategy.setState(() => [
			{
				decisions: [
					{
						id: `dec-${symbolStr}`,
						symbol: symbolStr,
						action,
						confidence,
						reason,
					},
				],
			},
		]);
		return;
	}

	if (source === "equity" || source === "balance") {
		const meta = (row as any).metadata || {};
		let cashVal = String(meta["cash"] || "");
		let unrealizedVal = String(meta["unrealized"] || "");
		let equityVal = String(meta["equity"] || "");
		updateEquity(cashVal, unrealizedVal, equityVal);
		return;
	}

	const sourceStr = (row.source as string) || "";
	const signalStore = signals[sourceStr as keyof typeof signals];
	if (!signalStore) {
		return;
	}

	let ring = signalStore.state[symbolStr as string];

	if (!ring) {
		ring = new RingBuffer<MeasurementT>(50);
		signalStore.state[symbolStr as string] = ring;
	}

	ring.add(row);
	
	if (source === "training") {
		signalStore.state[""] = ring;
		signalStore.state["learner"] = ring;
	}
	
	signals[sourceStr as keyof typeof signals]?.setState((prev: any) => ({ ...prev }));
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
					const row = decodeWireMeasurement(data.buffer);
					storeBatch(() => {
						dispatchMeasurement(row);
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
