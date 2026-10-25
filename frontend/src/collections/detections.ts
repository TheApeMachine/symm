import { createStore } from "@tanstack/store";
import {
	EXCURSION_KEYS,
	HINDSIGHT_DONE_KEYS,
	type HindsightFrame,
} from "#/collections/hindsight";
import { gobLayer } from "#/providers/gob-layer";

/*
Detections are what detect stored, streamed for the Historical runs tab
(data/schema.go, DetectionsOrigin) on the hub socket:

	{"type":"s3","prefix":"detections/"}        the epochs detect labelled
	{"type":"s3","prefix":"detections/","run":..}  that epoch's excursions
	{"type":"s3","prefix":"detections/<class>/<symbol>/","run":..,"symbol":..,"class":..,"at":"<c tick>"}
	                                            that excursion's trades A..D

Each stream closes on a done or an error frame; a new request of a slot
cancels the one in flight. Epochs and ticks are decimal strings: they exceed
what a JS number holds exactly.
*/

export const DETECTIONS_ORIGIN = "detections";

export const DETECTION_RUN_KEYS = [
	"first_tick",
	"last_tick",
	"ticks",
	"symbols",
	"up",
	"down",
	"chop",
	"flat",
] as const;

type Fields<K extends readonly string[]> = Record<K[number], number>;

export type DetectionRun = {
	epoch: string;
	fields: Fields<typeof DETECTION_RUN_KEYS>;
};

export type Detection = {
	symbol: string;
	class: string;
	fields: Fields<typeof EXCURSION_KEYS>;
};

/* One trade of an excursion's tape: detect's trade index, price, time (ns). */
export type DetectionTrade = { seq: number; price: number; at: number };

export type DetectionStatus = "streaming" | "done" | "error";

type Slot<T> = {
	status: DetectionStatus;
	error: string | null;
	items: T[];
};

export type DetectionsState = {
	runs: Slot<DetectionRun> | null;
	/* The excursions of one run. */
	excursions: (Slot<Detection> & { run: string }) | null;
	/* The trades of one excursion. */
	tape:
		| (Slot<DetectionTrade> & {
				run: string;
				symbol: string;
				class: string;
				cTick: string;
		  })
		| null;
};

export const detectionsStore = createStore<DetectionsState>({
	runs: null,
	excursions: null,
	tape: null,
});

const open = <T>(): Slot<T> => ({
	status: "streaming",
	error: null,
	items: [],
});

const DETECTION_CLASSES = ["up", "down", "chop", "flat"] as const;

type DetectionClass = (typeof DETECTION_CLASSES)[number];

const isDetectionClass = (role: string): role is DetectionClass =>
	(DETECTION_CLASSES as readonly string[]).includes(role);

const symbolsSeen = new Map<string, Set<string>>();

export const beginDetectionRuns = () => {
	flush();
	symbolsSeen.clear();
	detectionsStore.setState((state) => ({ ...state, runs: open() }));
	return { type: "s3", prefix: "detections/" };
};

export const beginDetections = (run: string) => {
	flush();
	detectionsStore.setState((state) => ({
		...state,
		excursions: { ...open<Detection>(), run },
		tape: null,
	}));
	return { type: "s3", prefix: "detections/", run };
};

export const beginDetectionTape = (
	run: string,
	symbol: string,
	cls: string,
	cTick: string,
) => {
	flush();
	detectionsStore.setState((state) => ({
		...state,
		tape: { ...open<DetectionTrade>(), run, symbol, class: cls, cTick },
	}));
	return {
		type: "s3",
		prefix: `detections/${cls}/${symbol}/`,
		run,
		symbol,
		class: cls,
		at: cTick,
	};
};

/*
abortDetections fails every detections stream still in flight (the command
was not sent, or the socket closed under it).
*/
export const abortDetections = (reason: string) => {
	flush();
	const fail = <S extends Slot<unknown> | null>(slot: S): S =>
		slot?.status === "streaming"
			? { ...slot, status: "error", error: reason }
			: slot;
	const state = detectionsStore.state;
	const inFlight = [state.runs, state.excursions, state.tape].some(
		(slot) => slot?.status === "streaming",
	);
	if (!inFlight) return;
	detectionsStore.setState((current) => ({
		runs: fail(current.runs),
		excursions: fail(current.excursions),
		tape: fail(current.tape),
	}));
	console.error("detections stream failed:", reason);
};

const labelsOf = (index: string, count: number) => {
	const at = index.indexOf(".bin/");
	const parts = (at >= 0 ? index.slice(at + ".bin/".length) : index)
		.split("/")
		.filter((part) => part.length > 0);
	return parts.length >= count ? parts.slice(parts.length - count) : parts;
};

const floats = (layer: { bytes: Uint8Array }) => {
	const decoded = gobLayer(layer.bytes);

	if (decoded?.kind === "floats") return decoded.floats;

	if (layer.bytes.byteLength % 8 !== 0) {
		throw new Error("detections: vector layer is not float64s");
	}
	const view = new DataView(
		layer.bytes.buffer,
		layer.bytes.byteOffset,
		layer.bytes.byteLength,
	);
	return Array.from({ length: layer.bytes.byteLength / 8 }, (_, idx) =>
		view.getFloat64(idx * 8, true),
	);
};

const readVector = <K extends readonly string[]>(
	frame: HindsightFrame,
	keys: K,
): Fields<K> => {
	if (frame.layers.length !== 1) {
		throw new Error(
			`detections: ${frame.role} frame holds ${frame.layers.length} layers, not 1`,
		);
	}
	const layer = frame.layers[0];
	const values = floats(layer);

	if (
		labelsOf(layer.index, keys.length).join("/") !== keys.join("/") ||
		values.length !== keys.length
	) {
		throw new Error(`detections: malformed ${frame.role} layer ${layer.index}`);
	}

	const fields = {} as Fields<K>;
	keys.forEach((key, idx) => {
		fields[key as K[number]] = values[idx];
	});
	return fields;
};

/*
slotOf names the request a frame answers: a detection_run frame the runs;
an excursion the run's excursions; trades the tape; a done or error frame
by its run (0: the runs) and symbol ("": the excursions).
*/
const slotOf = (frame: HindsightFrame): keyof DetectionsState => {
	if (frame.role === "detection_run") return "runs";
	if (frame.role === "detector") return "excursions";
	if (frame.role === "trades") return "tape";
	if (frame.epoch === 0n) return "runs";
	return frame.scope === "" ? "excursions" : "tape";
};

/*
Frames are applied at most every FLUSH_MS (an epoch's excursions or an
excursion's tape is many frames); a done or error frame applies at once.
*/
const FLUSH_MS = 50;
let pending: DetectionsState | null = null;
let flushTimer: ReturnType<typeof setTimeout> | null = null;

const flush = () => {
	if (flushTimer !== null) clearTimeout(flushTimer);
	flushTimer = null;
	if (pending === null) return;
	const next = pending;
	pending = null;
	detectionsStore.setState(() => next);
};

const blankRun = (): Fields<typeof DETECTION_RUN_KEYS> =>
	Object.fromEntries(DETECTION_RUN_KEYS.map((key) => [key, 0])) as Fields<
		typeof DETECTION_RUN_KEYS
	>;

/*
applyStoredDetection files one stored excursion. Its role is the class.
An open run list counts it; the open epoch lists it.
*/
const applyStoredDetection = (
	state: DetectionsState,
	frame: HindsightFrame,
	cls: DetectionClass,
) => {
	const fields = readVector(frame, EXCURSION_KEYS);
	const epoch = frame.epoch.toString();

	if (state.runs?.status === "streaming") {
		const items = state.runs.items;
		let row = items.find((item) => item.epoch === epoch);

		if (!row) {
			row = { epoch, fields: blankRun() };
			items.push(row);
		}

		row.fields[cls] += 1;
		const seen = symbolsSeen.get(epoch) ?? new Set<string>();
		seen.add(frame.scope);
		symbolsSeen.set(epoch, seen);
		row.fields.symbols = seen.size;
		if (row.fields.first_tick === 0 || fields.a_tick < row.fields.first_tick) {
			row.fields.first_tick = fields.a_tick;
		}
		if (fields.d_tick > row.fields.last_tick)
			row.fields.last_tick = fields.d_tick;
		row.fields.ticks =
			row.fields.up + row.fields.down + row.fields.chop + row.fields.flat;
	}

	if (
		state.excursions?.status === "streaming" &&
		state.excursions.run === epoch
	) {
		state.excursions.items.push({
			symbol: frame.scope,
			class: cls,
			fields,
		});
	}
};

const copySlots = (state: DetectionsState) => {
	for (const name of ["runs", "excursions", "tape"] as const) {
		const slot = state[name];
		if (slot)
			(state[name] as Slot<unknown>) = { ...slot, items: [...slot.items] };
	}
};

const ingestStoredDetection = (frame: HindsightFrame) => {
	if (!isDetectionClass(frame.role)) return;

	const fresh = pending === null;
	const state: DetectionsState = pending ?? { ...detectionsStore.state };
	const epoch = frame.epoch.toString();
	const runsOpen = state.runs?.status === "streaming";
	const excursionsOpen =
		state.excursions?.status === "streaming" && state.excursions.run === epoch;

	if (!runsOpen && !excursionsOpen) return;

	if (fresh) copySlots(state);
	pending = state;

	try {
		applyStoredDetection(state, frame, frame.role);
	} catch (cause) {
		const message = cause instanceof Error ? cause.message : String(cause);
		if (state.runs?.status === "streaming") {
			state.runs.error = message;
			state.runs.status = "error";
		}
		if (excursionsOpen && state.excursions) {
			state.excursions.error = message;
			state.excursions.status = "error";
		}
		console.error(message);
		flush();
		return;
	}

	flushTimer ??= setTimeout(flush, FLUSH_MS);
};

/*
ingestDetectionFrame applies one detections frame to the request it
answers. A frame of no open request (one since replaced) is stale and not
applied; a malformed frame fails its stream, loudly.
*/
export const ingestDetectionFrame = (frame: HindsightFrame) => {
	if (isDetectionClass(frame.role)) {
		ingestStoredDetection(frame);
		return;
	}

	const key = slotOf(frame);
	const fresh = pending === null;
	const state: DetectionsState = pending ?? { ...detectionsStore.state };
	const run = frame.epoch.toString();
	const current = state[key];

	if (current === null || current.status !== "streaming") return;
	if (key === "excursions" && state.excursions?.run !== run) return;
	if (
		key === "tape" &&
		(state.tape?.run !== run || state.tape.symbol !== frame.scope)
	)
		return;

	// Each slot's items are copied once per flush, then appended in place.
	if (fresh) copySlots(state);

	pending = state;
	const target = state[key] as Slot<unknown>;

	try {
		switch (frame.role) {
			case "detection_run":
				target.items.push({
					epoch: run,
					fields: readVector(frame, DETECTION_RUN_KEYS),
				});
				break;
			case "detector": {
				// The stored key: detector/<class>/<symbol>/<epoch>/<tick>.bin.
				const cls = frame.layers[0]?.index.split("/")[1] ?? "";
				target.items.push({
					symbol: frame.scope,
					class: cls,
					fields: readVector(frame, EXCURSION_KEYS),
				});
				break;
			}
			case "trades": {
				const [seqs, prices, ats] = ["seq", "price", "at"].map((name) => {
					const layer = frame.layers.find((entry) =>
						entry.index.endsWith(`/${name}`),
					);
					if (!layer) {
						throw new Error(
							`detections: trades frame without its ${name} layer`,
						);
					}
					return floats(layer);
				});
				if (prices.length !== seqs.length || ats.length !== seqs.length) {
					throw new Error("detections: trades layers of unequal length");
				}
				seqs.forEach((seq, idx) => {
					target.items.push({ seq, price: prices[idx], at: ats[idx] });
				});
				break;
			}
			case "done":
				readVector(frame, HINDSIGHT_DONE_KEYS);
				target.status = "done";
				flush();
				return;
			case "error":
				target.error = new TextDecoder().decode(frame.layers[0]?.bytes);
				target.status = "error";
				console.error("detections stream failed:", target.error);
				flush();
				return;
			default:
				throw new Error(`detections: unknown frame role ${frame.role}`);
		}
	} catch (cause) {
		target.error = cause instanceof Error ? cause.message : String(cause);
		target.status = "error";
		console.error(target.error);
		flush();
		return;
	}

	flushTimer ??= setTimeout(flush, FLUSH_MS);
};
