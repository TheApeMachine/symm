import {
	closedPositionsAtom,
	decisionsAtom,
	errorAtom,
	focusAtom,
	type Metric,
	type NamedNumber,
	observeSymbols,
	onlineAtom,
	positionCountAtom,
	positionsAtom,
	RingBuffer,
	routeAtom,
	signals,
	type Tag,
	tickCountAtom,
	updateClock,
	updateEquity,
	Value,
} from "#/collections/app";
import { DecisionT, HoldingT, PositionT } from "#/collections/types";
import { topologyStore } from "#/collections/topology";
import { hubWsUrl } from "#/lib/hub";
import { batch as storeBatch } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import { useEffect } from "react";

/*
The hub multiplexes everything over one socket: each message is one serialized
*data.Value, routed here by its layer index. Commands go back on the same socket.
*/
let wsWorker: Worker | null = null;

const toBackendRoute = (route: string) => {
	// The learning surface hosts the Impulse Map and Position Health inspector,
	// which require focused signal streams (CVD, Hawkes, Depthflow, etc.) to light up
	// the SOM grid confluence regions.
	return route === "learning" ? "dashboard" : route;
};

export const sendRoute = (route: string) => {
	wsWorker?.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
};

/*
Pending display state accumulated between animation frames. Ring buffers are
mutated as frames arrive; subscribers are notified once per display frame so a
burst of websocket messages costs one render, not one render per message.
*/
const pendingSources = new Set<string>();
let pendingAt: bigint | null = null;
let flushHandle: number | null = null;

const flush = () => {
	flushHandle = null;

	storeBatch(() => {
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
	epoch: bigint;
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
	if (remaining < 53) return null;

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
	const epoch = view.getBigUint64(40, true);

	let cursor = byteOffset + 48;

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
		epoch,
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


/*
SIGNAL_OF_KEY names the signal that owns a vector layer, by the first metric
key it emits. A tick Value carries every signal's layer under the tick's own
Prefix, so the index names the metrics but not the signal.
*/
const SIGNAL_OF_KEY: Record<string, string> = {
	cohort_covariance_score: "correlation",
	midpoint_log_return: "cvd",
	book_imbalance: "depthflow",
	"arrival_rate:buy": "hawkes",
	defined_peer_count: "leadlag",
	"best_price:bid": "liquidity",
	book_shape_distance: "morphology",
	spread_ratio: "pumpdump",
	breadth: "sentiment",
	"retreat_fraction:bid": "toxicity",
};

const parseJSON = (bytes: Uint8Array): unknown => {
	if (bytes[0] !== 0x7b && bytes[0] !== 0x5b) return undefined;
	try {
		return JSON.parse(textDecoder.decode(bytes));
	} catch {
		return undefined;
	}
};

const readFloats = (bytes: Uint8Array): number[] => {
	const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
	const floats: number[] = [];
	for (let j = 0; j < bytes.length / 8; j++) {
		floats.push(dv.getFloat64(j * 8, true));
	}
	return floats;
};

/*
tickSymbol is the symbol a Value describes: its Scope, or for an aggregate tick
the symbol named by its book/price/instrument layers or its trade frame.
*/
const tickSymbol = (val: ParsedValue): string => {
	if (val.scope && val.scope !== "aggregate") return val.scope;

	for (const layer of val.layers) {
		const match = /^(?:book|price|instrument)\/(.+)$/.exec(layer.index);
		if (match) return match[1];
	}

	for (const layer of val.layers) {
		if (layer.index.startsWith("spot:trade")) {
			const trade = parseJSON(layer.bytes) as
				| { data?: Array<{ symbol?: string }> }
				| undefined;
			return trade?.data?.[0]?.symbol ?? "";
		}
	}

	return "";
};

type Wallet = {
	data?: Array<{ asset?: string; balance?: string | number }>;
};

const applyBalance = (wallet: Wallet) => {
	const quote = wallet.data?.find((entry) => entry.asset === "USD");
	if (quote?.balance !== undefined) {
		updateEquity(String(quote.balance), null, null);
	}
};

type PositionLayer = {
	symbol?: string;
	status?: string;
	closed_at?: string;
	entry_price?: string | number;
	mark?: string | number;
};

const applyPositions = (rows: PositionLayer[]) => {
	const open: PositionT[] = [];
	const closed: HoldingT[] = [];

	for (const row of rows) {
		const holding = new HoldingT({
			symbol: row.symbol ?? "",
			status: row.status ?? "",
			entryPrice: row.entry_price ?? "0",
			mark: row.mark ?? "0",
		});

		if (row.closed_at) {
			closed.push(holding);
		} else {
			open.push(new PositionT({ status: holding.status, holding }));
		}
	}

	positionsAtom.set(open);
	closedPositionsAtom.set(closed);
	positionCountAtom.set(open.length);
};

type Reading = {
	source: string;
	symbol: string;
	metrics: Metric[];
	metadata: NamedNumber[];
	provenance: Tag[];
	snr: number;
	snrDefined: boolean;
	layers: Uint8Array[];
	index: string[];
};

const newReading = (source: string, symbol: string): Reading => ({
	source,
	symbol,
	metrics: [],
	metadata: [],
	provenance: [],
	snr: 0,
	snrDefined: false,
	layers: [],
	index: [],
});

/*
layerSymbol is the symbol a signal layer describes. Signals index their layer
as <symbol>/<Prefix>/<keys>, so one tick of a multi-symbol trade frame carries
one layer per symbol; a layer without that lead belongs to the tick symbol.
*/
const layerSymbol = (val: ParsedValue, index: string, fallback: string): string => {
	const head = index.split(".bin/")[0] ?? "";
	const at = head.indexOf(`/${val.origin}/`);
	return at > 0 ? head.slice(0, at) : fallback;
};

/*
readVector turns one vector layer into metrics: the labels after the Value's
Prefix name the floats in order, and any labels past the floats are tags
(action:<x>, path:<a/b>, ...).
*/
const readVector = (val: ParsedValue, index: string, bytes: Uint8Array) => {
	const floats = readFloats(bytes);
	const after = index.includes(".bin/") ? index.split(".bin/")[1] : "";
	const labels = after ? after.split("/") : [];
	const metrics: Metric[] = [];
	const provenance: Tag[] = [];

	if (labels.length === 0) {
		floats.forEach((f, k) => {
			metrics.push({ name: `m${k}`, raw: f, snm: f });
		});
	}

	for (let k = 0; k < labels.length; k++) {
		const label = labels[k];

		if (k < floats.length) {
			metrics.push({ name: label, raw: floats[k], snm: floats[k] });
			continue;
		}

		if (label.startsWith("path:")) {
			const path = labels.slice(k).join("/").slice("path:".length);
			provenance.push({ name: "token_path", value: path });
			break;
		}

		const [key, value = ""] = label.split(/:(.*)/s);
		provenance.push({
			name: key === "action" ? "decision_action" : key,
			value,
		});
	}

	const source =
		SIGNAL_OF_KEY[labels[0] ?? ""] ??
		(val.origin.split(":")[0] || "").toLowerCase();

	return { source, metrics, provenance };
};

/*
dispatchValue routes one Value's layers to their consumers by layer index:
spot:trade is the tick, balance and positions feed the desk atoms, and each
vector layer lands in its signal's ring for the tick's symbol.
*/
export function dispatchValue(val: ParsedValue) {
	const symbol = tickSymbol(val);

	if (symbol) {
		observeSymbols([symbol]);
	}

	if (val.tick > 0n) {
		tickCountAtom.set(Number(val.tick));
	}

	if (val.timestamp > 0n) {
		pendingAt = val.timestamp;
	}

	const readings = new Map<string, Reading>();
	let sawPositions = false;

	for (const layer of val.layers) {
		const { index, bytes } = layer;

		if (bytes.length === 0) continue;

		if (index === "balance") {
			applyBalance(parseJSON(bytes) as Wallet);
			continue;
		}

		if (index === "positions") {
			sawPositions = true;
			applyPositions((parseJSON(bytes) as PositionLayer[]) ?? []);
			continue;
		}

		if (
			index.startsWith("spot:") ||
			/^(?:book|price|instrument|position)(?:\/|$)/.test(index)
		) {
			continue;
		}

		if (bytes.length < 8 || bytes.length % 8 !== 0) continue;

		const { source, metrics, provenance } = readVector(val, index, bytes);
		const owner = layerSymbol(val, index, symbol);
		const key = `${source}\u0000${owner}`;
		const reading = readings.get(key) ?? newReading(source, owner);

		reading.metrics.push(...metrics);
		reading.provenance.push(...provenance);
		reading.layers.push(bytes);
		reading.index.push(index);

		for (const m of metrics) {
			reading.metadata.push({ name: m.name, value: m.raw });
			if (m.name === "snr") {
				reading.snr = m.raw;
				reading.snrDefined = true;
			}
		}

		readings.set(key, reading);
	}

	// Empty layers are skipped on the wire, so a tick without a positions
	// layer means nothing is held.
	if (val.role === "tick" && !sawPositions) {
		applyPositions([]);
	}

	for (const reading of readings.values()) {
		const { source, symbol } = reading;
		const value = new Value({
			id: val.id,
			origin: source,
			scope: symbol,
			role: val.role,
			tick: val.tick,
			seqIdx: val.seqIdx,
			timestamp: val.timestamp,
			layers: reading.layers,
			index: reading.index,
			metrics: reading.metrics,
			metadata: reading.metadata,
			provenance: reading.provenance,
			snr: reading.snr,
			snrDefined: reading.snrDefined,
		});

		topologyStore.actions.ingestValues([value]);

		let signalStore = signals[source];
		if (!signalStore) {
			signalStore = createStore<Record<string, RingBuffer<Value>>>({});
			signals[source] = signalStore;
		}

		let ring = signalStore.state[symbol];
		if (!ring) {
			ring = new RingBuffer<Value>(source === "training" ? 4096 : 50);
			signalStore.state[symbol] = ring;
		}

		ring.add(value);
		pendingSources.add(source);

		if (source === "training") {
			const tag = (name: string) =>
				reading.provenance.find((p) => p.name === name)?.value;
			const action = tag("decision_action");
			const cause = tag("token_path");

			if (action || cause) {
				const current = decisionsAtom.get().filter((d) => d.symbol !== symbol);
				current.push(
					new DecisionT({
						id: val.id || `dec-${symbol}`,
						symbol,
						action: action ?? "",
						cause: cause ?? "",
						reason: tag("reason") ?? "",
						confidence: reading.snrDefined ? reading.snr : 0,
					}),
				);
				decisionsAtom.set(current);
			}
		}
	}

	scheduleFlush();
}

export const WsFeed = () => {
	useEffect(() => {
		const url = `${hubWsUrl()}/ws`;
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

			if (data.type === "ERROR") {
				console.error("WS error:", data.error);
				errorAtom.set({ error: String(data.error), source: "WebSocket", url });
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
					for (const val of parseValues(new Uint8Array(data.buffer)) ?? []) {
						dispatchValue(val);
					}
				} catch (err) {
					console.error("WS message processing error:", err);
				}
			}
		});

		worker.postMessage({
			type: "ROUTE",
			route: toBackendRoute(routeAtom.get()),
		});
		worker.postMessage({ type: "CONNECT", url });
		wsWorker = worker;

		const unsubscribeFocus = focusAtom.subscribe((symbol: string) => {
			worker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = routeAtom.subscribe((route: string) => {
			worker.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			wsWorker = null;

			if (flushHandle !== null) {
				cancelAnimationFrame(flushHandle);
				flushHandle = null;
			}
		};
	}, []);

	return null;
};
