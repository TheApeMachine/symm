import { batch as storeBatch } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import { useEffect } from "react";
import {
	closedPositionsAtom,
	decisionsAtom,
	deskPositionsAtom,
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
import {
	COGNITION_ORIGIN,
	ingestCognitionFrame,
	ingestPolicyLayers,
	ingestTrainRegionsLayer,
	ingestTrainReject,
	LAYER_COGNITION,
} from "#/collections/cognition";
import {
	abortDetections,
	DETECTIONS_ORIGIN,
	ingestDetectionFrame,
} from "#/collections/detections";
import {
	abortHindsight,
	HINDSIGHT_ORIGIN,
	ingestHindsightFrame,
} from "#/collections/hindsight";
import {
	ingestPaperRecord,
	PAPER_ORIGIN,
	parsePaperRecord,
} from "#/collections/paper";
import { ingestSequiturFrame } from "#/collections/sequitur";
import { topologyStore } from "#/collections/topology";
import type { DeskPosition } from "#/collections/types";
import { DecisionT, HoldingT, PositionT } from "#/collections/types";
import { hubWsUrl } from "#/lib/hub";
import {
	ingestPositionFrame,
	LAYER_POSITION,
	POSITION_ORIGIN,
} from "#/lib/lifecycle";
import { gobLayer } from "#/providers/gob-layer";

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
sendCommand sends one JSON command the hub answers on this socket. With no
socket the command cannot be sent, which is an error, not a no-op.
*/
export const sendCommand = (command: Record<string, unknown>) => {
	if (wsWorker === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}

	wsWorker.postMessage({ type: "COMMAND", command });
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
parseSignalIndex reads a signal layer index (data.SignalIndex in data/schema.go):

	signal/<source>/<label...>/<tag...>/symbol:<symbol>

The symbol tag is last because a symbol carries a "/" itself. An index in the
signal namespace without a source or symbol tag is malformed: it is reported
and yields null, never a guessed owner.
*/
const SIGNAL_NAMESPACE = "signal/";
const SEQUITUR_ORIGIN = "sequitur";
const SEQUITUR_ROLE = "grammar";
const LAYER_GRAMMAR = "grammar";
const SYMBOL_TAG = "/symbol:";

export const parseSignalIndex = (
	index: string,
): { source: string; symbol: string; labels: string[] } | null => {
	if (!index.startsWith(SIGNAL_NAMESPACE)) return null;

	const at = index.lastIndexOf(SYMBOL_TAG);
	const head = at >= 0 ? index.slice(SIGNAL_NAMESPACE.length, at) : "";
	const symbol = at >= 0 ? index.slice(at + SYMBOL_TAG.length) : "";
	const [source = "", ...labels] = head.split("/");

	if (!source || !symbol) {
		console.error("parseSignalIndex: malformed signal layer index", index);
		return null;
	}

	return { source, symbol, labels };
};

const parseJSON = (bytes: Uint8Array): unknown => {
	if (bytes[0] !== 0x7b && bytes[0] !== 0x5b) return undefined;
	try {
		return JSON.parse(textDecoder.decode(bytes));
	} catch (err) {
		// A layer that opens like JSON but does not parse is corrupt data,
		// not an absent layer: raise it with the text that failed.
		console.error(
			"parseJSON: corrupt JSON layer",
			err,
			textDecoder.decode(bytes.subarray(0, 200)),
		);
		return undefined;
	}
};

const readFloats = (bytes: Uint8Array): number[] => {
	const decoded = gobLayer(bytes);

	if (decoded?.kind === "floats") return decoded.floats;

	if (bytes.length % 8 !== 0) return [];

	const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
	const floats: number[] = [];
	for (let j = 0; j < bytes.length / 8; j++) {
		floats.push(dv.getFloat64(j * 8, true));
	}
	return floats;
};

const layerBytes = (bytes: Uint8Array): Uint8Array => {
	const decoded = gobLayer(bytes);

	if (decoded?.kind === "bytes") return decoded.bytes;

	return bytes;
};

/*
layerAddress drops the Value prefix Append writes in front of every key.
*/
const layerAddress = (val: ParsedValue, index: string): string => {
	const prefix = `${val.origin}/${val.role}/${val.scope}/`;

	return index.startsWith(prefix) ? index.slice(prefix.length) : index;
};

/*
tickSymbol is the symbol a Value describes: its Scope, or for an aggregate tick
the symbol named by its book/price/instrument layers or its trade frame.
*/
const tickSymbol = (val: ParsedValue): string => {
	if (val.scope && val.scope !== "aggregate") return val.scope;

	for (const layer of val.layers) {
		const match = /^(?:book|price|instrument)\/(.+)$/.exec(
			layerAddress(val, layer.index),
		);
		if (match) return match[1];
	}

	for (const layer of val.layers) {
		if (layerAddress(val, layer.index).startsWith("spot:trade")) {
			const trade = parseJSON(layerBytes(layer.bytes)) as
				| { data?: Array<{ symbol?: string }> }
				| undefined;
			return trade?.data?.[0]?.symbol ?? "";
		}
	}

	return "";
};

type Wallet = {
	data?: Array<{ asset?: string; balance?: string | number }>;
	/* TradeBalanceResult: n is unrealized, e is equity. */
	n?: string | number;
	e?: string | number;
};

const moneyText = (value: string | number | null | undefined) => {
	if (value === null || value === undefined) return null;
	return String(value);
};

const applyBalance = (wallet: Wallet) => {
	const quote = wallet.data?.find((entry) => entry.asset === "USD");
	updateEquity(
		quote?.balance !== undefined ? String(quote.balance) : null,
		moneyText(wallet.n),
		moneyText(wallet.e),
	);
};

const decimalText = (value: unknown) => {
	if (typeof value === "string" && value !== "") return value;
	if (typeof value === "number" && Number.isFinite(value)) return String(value);
	return null;
};

type Touch = { bid: string | null; ask: string | null; feeRate: number | null };

const touches = new Map<string, Touch>();
const bidDepth = new Map<string, number>();

const rememberBook = (symbol: string, body: unknown) => {
	if (!body || typeof body !== "object") return;

	const levels = (body as { bids?: { levels?: Record<string, { quantity?: unknown }> } })
		.bids?.levels;

	if (!levels) return;

	let sum = 0;

	for (const level of Object.values(levels)) {
		const qty = decimalText(level?.quantity);
		const size = qty === null ? Number.NaN : Number(qty);

		if (Number.isFinite(size) && size > 0) sum += size;
	}

	if (sum > 0) bidDepth.set(symbol, sum);
};

const exitRoomOf = (row: Record<string, unknown>, symbol: string, qty: string | null) => {
	if (typeof row.exit_room === "number" && Number.isFinite(row.exit_room)) {
		return row.exit_room;
	}

	if (typeof row.exit_room === "string" && row.exit_room !== "") {
		const parsed = Number(row.exit_room);

		if (Number.isFinite(parsed)) return parsed;
	}

	const depth = bidDepth.get(symbol);
	const size = qty === null ? Number.NaN : Number(qty);

	if (depth === undefined || !Number.isFinite(size) || size <= 0) return null;

	return depth / size;
};

const rememberTouch = (symbol: string, body: unknown) => {
	if (!body || typeof body !== "object") return;

	const row = body as Record<string, unknown>;
	touches.set(symbol, {
		bid: decimalText(row.bid),
		ask: decimalText(row.ask),
		feeRate:
			typeof row.feeRate === "number" && Number.isFinite(row.feeRate)
				? row.feeRate
				: null,
	});
};

const touchFee = (touch: Touch | undefined, qty: string | null) => {
	if (!touch?.ask || touch.feeRate === null || !qty) return null;

	const ask = Number(touch.ask);
	const size = Number(qty);

	if (!Number.isFinite(ask) || !Number.isFinite(size)) return null;

	return String(ask * size * touch.feeRate);
};

const fillNumbers = (fills: unknown) => {
	let entry: string | null = null;
	let fee: string | null = null;

	if (!Array.isArray(fills)) return { entry, fee };

	for (const raw of fills) {
		if (!raw || typeof raw !== "object") continue;

		const fill = raw as Record<string, unknown>;
		const side = typeof fill.side === "string" ? fill.side.toLowerCase() : "";

		if (side === "sell") continue;

		entry ??= decimalText(fill.avg_price) ?? decimalText(fill.last_price);
		fee ??= decimalText(fill.fee_usd_equiv);
	}

	return { entry, fee };
};

/*
applyPositions reads the tick's positions layer. Each element is a
broker.Position. Entry and fee live on the buy fills when the flat fields
are empty. The mark is the price layer's bid, the sale touch.
*/
const applyPositions = (rows: unknown) => {
	const open: PositionT[] = [];
	const closed: HoldingT[] = [];
	const desk: DeskPosition[] = [];
	const list = Array.isArray(rows) ? rows : [];

	for (const raw of list) {
		if (!raw || typeof raw !== "object") continue;

		const row = raw as Record<string, unknown>;

		if (typeof row.symbol !== "string" || row.symbol === "") continue;

		const closedAt = typeof row.closed_at === "string" ? row.closed_at : null;
		const fromFills = fillNumbers(row.fills);
		const orders = Array.isArray(row.entry_orders) ? row.entry_orders : [];
		const firstOrder =
			orders[0] && typeof orders[0] === "object"
				? (orders[0] as Record<string, unknown>)
				: undefined;
		const touch = touches.get(row.symbol);
		const qty = decimalText(row.qty) ?? decimalText(firstOrder?.volume);
		const position: DeskPosition = {
			id: typeof row.id === "string" ? row.id : row.symbol,
			symbol: row.symbol,
			virtual: row.virtual === true,
			openedAt: typeof row.opened_at === "string" ? row.opened_at : "",
			closedAt,
			qty,
			entryPrice: decimalText(row.entry_price) ?? fromFills.entry ?? touch?.ask ?? null,
			mark: decimalText(row.mark) ?? touch?.bid ?? null,
			entryFee: decimalText(row.entry_fee) ?? fromFills.fee ?? touchFee(touch, qty),
			exitFee: decimalText(row.exit_fee),
			exitVwap: decimalText(row.exit_vwap),
			exitRoom: exitRoomOf(row, row.symbol, qty),
			returnPct: typeof row.return === "number" ? row.return : null,
			holdNs: typeof row.hold_ns === "number" ? row.hold_ns : 0,
		};

		desk.push(position);

		if (position.virtual) continue;

		const holding = new HoldingT({
			symbol: position.symbol,
			status: closedAt ? "closed" : "open",
			entryPrice: position.entryPrice ?? "0",
			mark: position.mark ?? "0",
		});

		if (closedAt) {
			closed.push(holding);
		} else {
			open.push(new PositionT({ status: holding.status, holding }));
		}
	}

	deskPositionsAtom.set(desk);
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
readVector turns one vector layer into metrics: the labels after the Value's
Prefix name the floats in order, and any labels past the floats are tags
(action:<x>, path:<a/b>, ...).
*/
const readVector = (labels: string[], floats: number[]) => {
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

	return { metrics, provenance };
};

/*
dispatchValue routes one Value's layers to their consumers by layer index:
spot:trade is the tick, balance and positions feed the desk atoms, and each
vector layer lands in its signal's ring for the tick's symbol.
*/
export function dispatchValue(val: ParsedValue) {
	if (val.origin === "trace") {
		const layer = val.layers.find((entry) => entry.index === "trace");

		if (layer) {
			try {
				topologyStore.actions.ingestTrace(
					JSON.parse(textDecoder.decode(layer.bytes)),
				);
			} catch (err) {
				console.error("trace frame did not parse", err);
			}
		}

		return;
	}
	// A live grammar frame (sequitur/grammar/<symbol>, data/schema.go) is not
	// a tick: its one layer is the strategy.SequiturFrame JSON.
	if (val.origin === SEQUITUR_ORIGIN && val.role === SEQUITUR_ROLE) {
		const layer = val.layers.find((entry) => entry.index === LAYER_GRAMMAR);

		if (!layer) {
			console.error(
				"dispatchValue: sequitur frame without a grammar layer",
				val.scope,
			);
			return;
		}

		ingestSequiturFrame(parseJSON(layer.bytes));
		return;
	}

	// The loaded CognitionTree and live matches against it (cognition/<role>).
	if (val.origin === COGNITION_ORIGIN) {
		const layer = val.layers.find((entry) => entry.index === LAYER_COGNITION);

		if (!layer) {
			console.error(
				"dispatchValue: cognition frame without its layer",
				val.role,
				val.scope,
			);
			return;
		}

		ingestCognitionFrame(val.role, parseJSON(layer.bytes));
		return;
	}

	// A hindsight frame replays a stored run to this client only.
	if (val.origin === HINDSIGHT_ORIGIN) {
		ingestHindsightFrame(val);
		return;
	}

	// A detections frame streams what detect stored (Historical runs) to
	// this client only.
	if (val.origin === DETECTIONS_ORIGIN) {
		ingestDetectionFrame(val);
		return;
	}

	// A paper position frame (position/<event>/<symbol>, data/schema.go):
	// its one layer is the strategy.PositionFrame JSON.
	if (val.origin === POSITION_ORIGIN) {
		const layer = val.layers.find((entry) =>
			entry.index.endsWith(`/${LAYER_POSITION}`),
		);

		if (!layer) {
			console.error(
				"dispatchValue: position frame without its layer",
				val.role,
				val.scope,
			);
			return;
		}

		ingestPositionFrame(parseJSON(layer.bytes));
		return;
	}

	// A paper-trade record (paper/<action>/<symbol>, data.PaperKeys) is one
	// vector layer with its matched path and policy epoch tags.
	if (val.origin === PAPER_ORIGIN) {
		ingestPaperRecord(parsePaperRecord(val.role, val.scope, val.layers));
		return;
	}

	if (val.origin === "policy") {
		ingestPolicyLayers(val.epoch, val.layers);
	}

	for (const layer of val.layers) {
		if (layer.index.includes("train/regions/")) {
			ingestTrainRegionsLayer(
				Number(val.tick),
				layer.index,
				readFloats(layer.bytes),
			);
		}

		if (layer.index.includes("train/reject/")) {
			ingestTrainReject(Number(val.tick), layer.index);
		}
	}

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
	let positionPayload: unknown;

	for (const layer of val.layers) {
		const { bytes } = layer;
		const index = layerAddress(val, layer.index);

		if (bytes.length === 0) continue;

		if (index === "balance") {
			applyBalance(parseJSON(layerBytes(bytes)) as Wallet);
			continue;
		}

		if (index === "positions") {
			sawPositions = true;
			positionPayload = parseJSON(layerBytes(bytes));
			continue;
		}

		if (index.startsWith("price/")) {
			rememberTouch(index.slice("price/".length), parseJSON(layerBytes(bytes)));
			continue;
		}

		if (index.startsWith("book/")) {
			rememberBook(index.slice("book/".length), parseJSON(layerBytes(bytes)));
			continue;
		}

		if (
			index.startsWith("spot:") ||
			/^(?:book|price|instrument|position)(?:\/|$)/.test(index)
		) {
			continue;
		}

		const floats = readFloats(bytes);

		if (floats.length === 0) continue;

		// A signal layer names its source and symbol (parseSignalIndex); any
		// other vector layer belongs to the Value's own origin and symbol,
		// its labels after the Value's key (".bin/").
		let source: string;
		let owner: string;
		let labels: string[];

		if (index.startsWith(SIGNAL_NAMESPACE)) {
			const signal = parseSignalIndex(index);
			if (!signal) continue;
			({ source, symbol: owner, labels } = signal);
		} else {
			const after = index.includes(".bin/") ? index.split(".bin/")[1] : "";
			labels = after ? after.split("/") : [];
			source = (val.origin.split(":")[0] || "").toLowerCase();
			owner = symbol;
		}

		const { metrics, provenance } = readVector(labels, floats);
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
	// layer means nothing is held. Price layers in the same tick are the marks.
	if (sawPositions) {
		applyPositions(positionPayload ?? []);
	} else if (val.role === "tick") {
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

			if (data.type === "COMMAND_FAILED") {
				console.error("hub command not sent:", data.command, data.error);
				if (data.command?.type === "hindsight")
					abortHindsight(String(data.error));
				if (data.command?.type === "detections")
					abortDetections(String(data.error));
				return;
			}

			if (data.type === "STATUS") {
				onlineAtom.set(data.status);
				// The hub ends a replay with its client.
				if (data.status === "OFFLINE") {
					abortHindsight("hub socket closed");
					abortDetections("hub socket closed");
				}
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
