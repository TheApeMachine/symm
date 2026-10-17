import { createStore } from "@tanstack/store";
import { type PaperRecord, parsePaperRecord } from "#/collections/paper";

/*
Hindsight replays one stored run (data/schema.go, HindsightOrigin): the
client sends {"type":"hindsight","run":"<epoch>","symbol":...} on the hub
socket and the hub streams that client hindsight/<role>/<symbol> Values,
closed by a done frame or an error frame. A new request cancels the one in
flight, so the run index (symbol "") and a symbol's stream are requested
one after the other. The run is a decimal string: an epoch exceeds what a
JS number holds exactly.
*/

export const HINDSIGHT_ORIGIN = "hindsight";

export const HINDSIGHT_TICK_KEYS = [
	"bid",
	"ask",
	"trade_price",
	"trade_qty",
	"trades",
] as const;

export const HINDSIGHT_SYMBOL_KEYS = [
	"ticks",
	"touched",
	"trades",
	"first_tick",
	"last_tick",
	"first_at",
	"last_at",
	"low",
	"high",
	"up",
	"down",
	"top_excursion",
] as const;

export const HINDSIGHT_DONE_KEYS = [
	"ticks",
	"symbols",
	"excursions",
	"policies",
	"papers",
] as const;

export const EXCURSION_KEYS = [
	"a_idx",
	"a_tick",
	"b_idx",
	"b_tick",
	"c_idx",
	"c_tick",
	"d_idx",
	"d_tick",
	"b_price",
	"c_price",
	"b_ask",
	"c_bid",
	"b_at",
	"c_at",
	"entry_idx",
	"entry_tick",
	"exit_idx",
	"exit_tick",
	"entry_ask",
	"exit_bid",
	"entry_at",
	"exit_at",
] as const;

export const POLICY_KEYS = [
	"gain",
	"hold_seconds",
	"visits",
	"mean_reward",
	"confidence",
] as const;

type Fields<K extends readonly string[]> = Record<K[number], number>;

export type HindsightTick = {
	tick: number;
	/* Unix ms the tick was stored with. */
	at: number;
	fields: Fields<typeof HINDSIGHT_TICK_KEYS>;
};

export type HindsightExcursion = {
	/* up, down, chop or flat: the detector class its key names. */
	kind: string;
	key: string;
	fields: Fields<typeof EXCURSION_KEYS>;
};

export type HindsightPolicy = {
	action: string;
	path: string;
	fields: Fields<typeof POLICY_KEYS>;
};

export type HindsightSymbolRow = {
	symbol: string;
	fields: Fields<typeof HINDSIGHT_SYMBOL_KEYS>;
};

export type HindsightStatus = "streaming" | "done" | "error";

export type HindsightStream = {
	run: string;
	symbol: string;
	status: HindsightStatus;
	error: string | null;
	ticks: HindsightTick[];
	excursions: HindsightExcursion[];
	policies: HindsightPolicy[];
	papers: PaperRecord[];
	symbols: HindsightSymbolRow[];
	counts: Fields<typeof HINDSIGHT_DONE_KEYS> | null;
};

export type HindsightState = {
	/* The run index: symbol "". */
	index: HindsightStream | null;
	/* One symbol of the run. */
	stream: HindsightStream | null;
};

export const hindsightStore = createStore<HindsightState>({
	index: null,
	stream: null,
});

type Layer = { index: string; bytes: Uint8Array };

export type HindsightFrame = {
	epoch: bigint;
	tick: bigint;
	timestamp: bigint;
	role: string;
	scope: string;
	layers: Layer[];
};

const emptyStream = (run: string, symbol: string): HindsightStream => ({
	run,
	symbol,
	status: "streaming",
	error: null,
	ticks: [],
	excursions: [],
	policies: [],
	papers: [],
	symbols: [],
	counts: null,
});

/*
beginHindsight opens the slot for run/symbol and returns the hub command to
send for it. Frames of any other request are stale and are not applied.
*/
export const beginHindsight = (run: string, symbol: string) => {
	settle(true);
	const slot = symbol === "" ? "index" : "stream";
	hindsightStore.setState((state) => ({
		...state,
		[slot]: emptyStream(run, symbol),
	}));
	return { type: "hindsight", run, symbol };
};

/*
abortHindsight fails every stream still in flight, e.g. when the hub socket
closed under it: the hub ends a replay with its client.
*/
export const abortHindsight = (reason: string) => {
	settle(true);
	hindsightStore.setState((state) => {
		const fail = (stream: HindsightStream | null) =>
			stream?.status === "streaming"
				? { ...stream, status: "error" as const, error: reason }
				: stream;
		return { index: fail(state.index), stream: fail(state.stream) };
	});
};

/*
readVector reads a vector layer whose labels after ".bin/" are exactly keys
(then tags, when tagged). Anything else is malformed: an Error.
*/
const readVector = <K extends readonly string[]>(
	layer: Layer,
	keys: K,
	tagged = false,
): Fields<K> => {
	const at = layer.index.indexOf(".bin/");
	const labels =
		at >= 0 ? layer.index.slice(at + ".bin/".length).split("/") : [];
	const head = labels.slice(0, keys.length);

	if (
		head.join("/") !== keys.join("/") ||
		(!tagged && labels.length !== keys.length) ||
		layer.bytes.byteLength !== keys.length * 8
	) {
		throw new Error(`hindsight: malformed vector layer ${layer.index}`);
	}

	const view = new DataView(
		layer.bytes.buffer,
		layer.bytes.byteOffset,
		layer.bytes.byteLength,
	);
	const fields = {} as Fields<K>;

	keys.forEach((key, idx) => {
		fields[key as K[number]] = view.getFloat64(idx * 8, true);
	});

	return fields;
};

const onlyLayer = (frame: HindsightFrame): Layer => {
	if (frame.layers.length !== 1) {
		throw new Error(
			`hindsight: ${frame.role} frame for ${frame.scope} holds ${frame.layers.length} layers, not 1`,
		);
	}

	return frame.layers[0];
};

/* The stored key's second segment: the class or action it was written under. */
const keyRole = (layer: Layer) => layer.index.split("/")[1] ?? "";

const PATH_TAG = "/path:";

/*
Pending frames are applied to the store at most every FLUSH_MS, so a replay
of tens of thousands of ticks is not one render per frame.
*/
const FLUSH_MS = 50;
let pending: HindsightState | null = null;
let flushTimer: ReturnType<typeof setTimeout> | null = null;

const flush = () => {
	flushTimer = null;
	if (pending === null) return;
	const next = pending;
	pending = null;
	hindsightStore.setState(() => next);
};

const settle = (now: boolean) => {
	if (now) {
		if (flushTimer !== null) clearTimeout(flushTimer);
		flush();
		return;
	}

	flushTimer ??= setTimeout(flush, FLUSH_MS);
};

/*
ingestHindsightFrame applies one hindsight frame to the stream it belongs to.
A frame for no open request (a replay the hub has since canceled) is stale
and not applied. A malformed frame fails its stream, loudly.
*/
export const ingestHindsightFrame = (frame: HindsightFrame) => {
	const state = pending ?? {
		index: hindsightStore.state.index && { ...hindsightStore.state.index },
		stream: hindsightStore.state.stream && { ...hindsightStore.state.stream },
	};
	const slot = frame.scope === "" ? "index" : "stream";
	const run = frame.epoch.toString();
	const target = state[slot];

	if (
		target === null ||
		target.run !== run ||
		target.symbol !== frame.scope ||
		target.status !== "streaming"
	) {
		return;
	}

	// Arrays are copied once per flush, then appended in place.
	if (pending === null) {
		for (const key of [
			"ticks",
			"excursions",
			"policies",
			"papers",
			"symbols",
		] as const) {
			(target[key] as unknown[]) = [...target[key]];
		}
	}

	pending = state;

	try {
		switch (frame.role) {
			case "tick":
				target.ticks.push({
					tick: Number(frame.tick),
					at: Number(frame.timestamp / 1_000_000n),
					fields: readVector(onlyLayer(frame), HINDSIGHT_TICK_KEYS),
				});
				break;
			case "detector":
			case "draft":
			case "detections": {
				const layer = onlyLayer(frame);
				target.excursions.push({
					kind: keyRole(layer),
					key: layer.index.slice(0, layer.index.indexOf(".bin/") + 4),
					fields: readVector(layer, EXCURSION_KEYS),
				});
				break;
			}
			case "policy": {
				const layer = onlyLayer(frame);
				const at = layer.index.indexOf(PATH_TAG);
				const path = at >= 0 ? layer.index.slice(at + PATH_TAG.length) : "";

				if (path === "")
					throw new Error(`hindsight: policy ${layer.index} has no path tag`);

				target.policies.push({
					action: keyRole(layer),
					path,
					fields: readVector(layer, POLICY_KEYS, true),
				});
				break;
			}
			case "paper": {
				const record = parsePaperRecord(
					keyRole(onlyLayer(frame)),
					frame.scope,
					frame.layers,
				);
				if (record === null)
					throw new Error(
						`hindsight: malformed paper record for ${frame.scope}`,
					);
				target.papers.push(record);
				break;
			}
			case "symbol": {
				// The run index frames carry their symbol in the key, Scope ""
				// is the request: the summary's symbol is its key's scope.
				// A later frame for the same symbol replaces the first, which
				// was sent when the symbol was first seen.
				const row = readSymbolRow(frame);
				const at = target.symbols.findIndex(
					(item) => item.symbol === row.symbol,
				);
				if (at >= 0) target.symbols[at] = row;
				else target.symbols.push(row);
				break;
			}
			case "done":
				target.counts = readVector(onlyLayer(frame), HINDSIGHT_DONE_KEYS);
				target.status = "done";
				settle(true);
				return;
			case "error":
				target.error = new TextDecoder().decode(onlyLayer(frame).bytes);
				target.status = "error";
				console.error("hindsight replay failed:", target.error);
				settle(true);
				return;
			default:
				throw new Error(`hindsight: unknown frame role ${frame.role}`);
		}
	} catch (cause) {
		target.status = "error";
		target.error = cause instanceof Error ? cause.message : String(cause);
		console.error(target.error);
		settle(true);
		return;
	}

	settle(false);
};

/*
readSymbolRow reads a run index summary: its key is
hindsight/symbol/<symbol>/<run>/<tick>.bin, so the symbol sits between the
role and the run.
*/
const readSymbolRow = (frame: HindsightFrame): HindsightSymbolRow => {
	const layer = onlyLayer(frame);
	const key = layer.index.slice(0, layer.index.indexOf(".bin/"));
	const segments = key.split("/");
	const symbol = segments.slice(2, -2).join("/");

	if (symbol === "")
		throw new Error(`hindsight: summary ${layer.index} names no symbol`);

	return { symbol, fields: readVector(layer, HINDSIGHT_SYMBOL_KEYS) };
};
