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
import { DecisionT } from "#/collections/types";
import { topologyStore } from "#/collections/topology";
import { type Metric, type NamedNumber, type Tag, Value } from "#/types/value";
import { batch as storeBatch } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import { useEffect } from "react";

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

const toBackendRoute = (route: string) => {
	// The learning surface hosts the Impulse Map and Position Health inspector,
	// which require focused signal streams (CVD, Hawkes, Depthflow, etc.) to light up
	// the SOM grid confluence regions.
	return route === "learning" ? "dashboard" : route;
};

export const sendRoute = (route: string) => {
	const backendRoute = toBackendRoute(route);
	for (const worker of globalWsWorkers) {
		worker.postMessage({
			type: "ROUTE",
			route: backendRoute,
		});
	}
};

const defaultWsUrls = (): string[] => {
	if (typeof window === "undefined") {
		return ["ws://127.0.0.1:8765/ws/0", "ws://127.0.0.1:8765/ws/1"];
	}
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host = window.location.hostname || "127.0.0.1";
	return [`${protocol}//${host}:8765/ws/0`, `${protocol}//${host}:8765/ws/1`];
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

const textDecoder = new TextDecoder();

export interface ParsedValue {
	id: string;
	tick: bigint;
	seqIdx: bigint;
	timestamp: bigint;
	origin: string;
	role: string;
	scope: string;
	layers: Array<{ index: string; bytes: Uint8Array }>;
	byteLength: number;
}

export const parseOneValue = (
	bytes: Uint8Array,
	byteOffset = 0,
): ParsedValue | null => {
	const remaining = bytes.byteLength - byteOffset;
	if (remaining < 45) return null;

	const view = new DataView(
		bytes.buffer,
		bytes.byteOffset + byteOffset,
		remaining,
	);

	const idBytes = bytes.subarray(byteOffset, byteOffset + 16);
	const id = Array.from(idBytes)
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");

	const tick = view.getBigUint64(16, true);
	const seqIdx = view.getBigUint64(24, true);
	const timestamp = view.getBigUint64(32, true);

	let cursor = byteOffset + 40;

	if (cursor >= bytes.byteLength) return null;
	const originLen = bytes[cursor++];
	if (cursor + originLen > bytes.byteLength) return null;
	const origin = textDecoder.decode(bytes.subarray(cursor, cursor + originLen));
	cursor += originLen;

	if (cursor >= bytes.byteLength) return null;
	const roleLen = bytes[cursor++];
	if (cursor + roleLen > bytes.byteLength) return null;
	const role = textDecoder.decode(bytes.subarray(cursor, cursor + roleLen));
	cursor += roleLen;

	if (cursor >= bytes.byteLength) return null;
	const scopeLen = bytes[cursor++];
	if (cursor + scopeLen > bytes.byteLength) return null;
	const scope = textDecoder.decode(bytes.subarray(cursor, cursor + scopeLen));
	cursor += scopeLen;

	if (cursor + 2 > bytes.byteLength) return null;
	const numLayers = view.getUint16(cursor - byteOffset, true);
	cursor += 2;

	const layers: Array<{ index: string; bytes: Uint8Array }> = [];
	for (let i = 0; i < numLayers; i++) {
		if (cursor + 2 > bytes.byteLength) return null;
		const idxLen = view.getUint16(cursor - byteOffset, true);
		cursor += 2;
		if (cursor + idxLen > bytes.byteLength) return null;
		const index =
			idxLen > 0
				? textDecoder.decode(bytes.subarray(cursor, cursor + idxLen))
				: "";
		cursor += idxLen;

		if (cursor + 4 > bytes.byteLength) return null;
		const layerLen = view.getUint32(cursor - byteOffset, true);
		cursor += 4;
		if (cursor + layerLen > bytes.byteLength) return null;
		const layerBytes = bytes.subarray(cursor, cursor + layerLen);
		cursor += layerLen;

		layers.push({ index, bytes: layerBytes });
	}

	const totalSize = cursor - byteOffset;
	return {
		id,
		tick,
		seqIdx,
		timestamp,
		origin,
		role,
		scope,
		layers,
		byteLength: totalSize,
	};
};

export const parseValues = (bytes: Uint8Array): ParsedValue[] | null => {
	if (bytes.byteLength < 45) return null;

	const list: ParsedValue[] = [];
	let offset = 0;

	while (offset < bytes.byteLength) {
		const val = parseOneValue(bytes, offset);
		if (!val || val.byteLength === 0) {
			return null;
		}
		list.push(val);
		offset += val.byteLength;
	}

	return list.length > 0 ? list : null;
};

export function dispatchValue(val: ParsedValue) {
	const rawSource = (val.origin || "").toLowerCase();
	const source = rawSource.includes(":") ? rawSource.split(":")[0] : rawSource;
	const symbol = val.scope || "";

	if (symbol && !symbolsAtom.get().includes(symbol)) {
		observeSymbols([symbol]);
	}

	if (val.tick > 0n) {
		pendingTick = val.tick;
	}
	if (val.timestamp > 0n) {
		pendingAt = val.timestamp;
	}

	if (
		source === "spot" ||
		rawSource === "spot:trade" ||
		source === "spot:trade" ||
		val.role === "trade"
	) {
		tickCountAtom.set(Number(val.tick));
		scheduleFlush();
		return;
	}

	let cash = "";
	let unrealized = "";
	let equity = "";

	const metrics: Metric[] = [];
	const metadata: NamedNumber[] = [];
	const provenance: Tag[] = [];
	let snr = 0.0;
	let snrDefined = false;

	const prefix = `${val.origin}/${val.role}/${val.scope}/${String(val.tick).padStart(19, "0")}/${String(val.seqIdx).padStart(19, "0")}.bin`;

	for (const layer of val.layers) {
		const bytes = layer.bytes;
		if (bytes.length === 0) continue;

		// JSON payload check
		if (bytes[0] === 0x7b || bytes[0] === 0x5b) {
			try {
				const jsonStr = textDecoder.decode(bytes);
				const data = JSON.parse(jsonStr);

				if (data.cash !== undefined) cash = String(data.cash);
				if (data.unrealized !== undefined) unrealized = String(data.unrealized);
				if (data.equity !== undefined) equity = String(data.equity);

				if (data.price !== undefined) {
					metrics.push({
						name: "price",
						raw: Number(data.price),
						snm: Number(data.price),
					});
				} else if (
					Array.isArray(data.data) &&
					data.data[0]?.price !== undefined
				) {
					metrics.push({
						name: "price",
						raw: Number(data.data[0].price),
						snm: Number(data.data[0].price),
					});
				}

				if (data.rows && Array.isArray(data.rows)) {
					positionsAtom.set(data.rows);
					if (data.closed && Array.isArray(data.closed)) {
						closedPositionsAtom.set(data.closed);
					}
					positionCountAtom.set(data.rows.length);
				}

				for (const [k, v] of Object.entries(data)) {
					if (typeof v === "number") {
						metrics.push({ name: k, raw: v, snm: v });
						metadata.push({ name: k, value: v });
					} else if (
						typeof v === "string" &&
						!["cash", "unrealized", "equity", "rows", "closed"].includes(k)
					) {
						if (k === "action" || k === "decision_action") {
							provenance.push({ name: "decision_action", value: v });
						} else if (k === "path" || k === "token_path") {
							provenance.push({ name: "token_path", value: v });
						} else if (k === "reason" || k === "decision_reason") {
							provenance.push({ name: "decision_reason", value: v });
						}
						provenance.push({ name: k, value: v });
					}
				}
			} catch {
				// ignore JSON parse error
			}
			continue;
		}

		// Float64 vector payload check
		if (bytes.length >= 8 && bytes.length % 8 === 0) {
			const floatCount = bytes.length / 8;
			const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
			const floats: number[] = [];
			for (let j = 0; j < floatCount; j++) {
				floats.push(dv.getFloat64(j * 8, true));
			}

			let afterPrefix = layer.index;
			if (layer.index.includes(".bin/")) {
				afterPrefix = layer.index.split(".bin/")[1];
			} else if (prefix && layer.index.startsWith(prefix)) {
				afterPrefix = layer.index.slice(prefix.length).replace(/^\/+/, "");
			} else {
				const parts = layer.index.split("/");
				if (parts.length > 5) {
					afterPrefix = parts.slice(5).join("/");
				}
			}

			if (afterPrefix) {
				const labels = afterPrefix.split("/");
				for (let k = 0; k < labels.length; k++) {
					const label = labels[k];
					if (k < floats.length) {
						const f = floats[k];
						metrics.push({ name: label, raw: f, snm: f });
						if (label === "snr") {
							snr = f;
							snrDefined = true;
						}
					} else {
						if (label.startsWith("path:")) {
							const remainingSegments = labels.slice(k).join("/");
							const pathVal = remainingSegments.slice("path:".length);
							provenance.push({ name: "token_path", value: pathVal });
							provenance.push({ name: "path", value: pathVal });
							break;
						} else if (label.includes(":")) {
							const colonIdx = label.indexOf(":");
							const key = label.slice(0, colonIdx);
							const valStr = label.slice(colonIdx + 1);
							if (key === "action") {
								provenance.push({ name: "decision_action", value: valStr });
							} else if (key === "reason") {
								provenance.push({ name: "decision_reason", value: valStr });
							}
							provenance.push({ name: key, value: valStr });
						} else {
							provenance.push({ name: label, value: "" });
						}
					}
				}
			} else {
				for (let k = 0; k < floats.length; k++) {
					metrics.push({ name: `m${k}`, raw: floats[k], snm: floats[k] });
				}
			}
		}
	}

	if (source === "balance" || val.role === "balance") {
		if (cash || unrealized || equity) {
			updateEquity(cash, unrealized, equity);
		}
		return;
	}

	if (cash || unrealized || equity) {
		updateEquity(cash, unrealized, equity);
	}

	const value = new Value({
		id: val.id,
		origin: val.origin,
		scope: val.scope,
		role: val.role,
		tick: val.tick,
		seqIdx: val.seqIdx,
		timestamp: val.timestamp,
		layers: val.layers.map((l) => l.bytes),
		index: val.layers.map((l) => l.index),
		metrics,
		metadata,
		provenance,
		snr,
		snrDefined,
	});

	topologyStore.actions.ingestValues([value]);

	let signalStore = signals[source];
	if (!signalStore) {
		signalStore = createStore<Record<string, RingBuffer<Value>>>({});
		signals[source] = signalStore;
	}

	let ring = signalStore.state[symbol];
	if (!ring) {
		const capacity = source === "training" ? 4096 : 50;
		ring = new RingBuffer<Value>(capacity);
		signalStore.state[symbol] = ring;
	}

	ring.add(value);
	pendingSources.add(source);

	if (source === "training") {
		const actionProv = provenance.find(
			(p) => p.name === "decision_action" || p.name === "action",
		);
		const pathProv = provenance.find(
			(p) => p.name === "token_path" || p.name === "path",
		);
		if (actionProv || pathProv) {
			const current = decisionsAtom.get().filter((d) => d.symbol !== symbol);
			current.push(
				new DecisionT({
					id: val.id || `dec-${symbol}`,
					symbol,
					action: actionProv?.value ?? "",
					cause: pathProv?.value ?? "",
					reason:
						provenance.find(
							(p) => p.name === "decision_reason" || p.name === "reason",
						)?.value ?? "",
					confidence: snrDefined ? snr : 0,
				}),
			);
			decisionsAtom.set(current);
		}
	}

	if (pendingSources.size > 0 || pendingTick !== null || pendingAt !== null) {
		scheduleFlush();
	}
}

/*
dispatchValuesBuffer accepts serialized *data.Value bytes.
*/
export const dispatchValuesBuffer = (
	buffer: Uint8Array | ArrayBuffer | { bytes: () => Uint8Array },
) => {
	const bytes =
		buffer instanceof Uint8Array
			? buffer
			: buffer instanceof ArrayBuffer
				? new Uint8Array(buffer)
				: buffer.bytes();
	const values = parseValues(bytes);
	if (values && values.length > 0) {
		for (const val of values) {
			dispatchValue(val);
		}
	}
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
						errorAtom.set(null);
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
						const values = parseValues(bytes);
						if (values && values.length > 0) {
							for (const val of values) {
								dispatchValue(val);
							}
						}
					} catch (err) {
						console.error(`WS[${shardIndex}] message processing error:`, err);
					}
				}
			});

			worker.postMessage({
				type: "ROUTE",
				route: toBackendRoute(routeAtom.get()),
			});
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
			const backendRoute = toBackendRoute(route);
			for (const worker of workers) {
				worker.postMessage({ type: "ROUTE", route: backendRoute });
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
