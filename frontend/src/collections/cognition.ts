import { createStore } from "@tanstack/store";
import { trySendCommand } from "#/lib/hub-socket";
import { tryDecodeJsonBytes } from "#/lib/json-bytes";
import { parseOneValue } from "#/types/decoder";
import type { Value } from "#/types/value";

/*
The live symbol set, as known to the app (it also seeds the symbol
picker). Training indexes embed the symbol — "BTC/USD" is two "/" segments
— and the known set resolves the symbol boundary unambiguously at the
segment level.
*/
export const SYMBOLS = [
	"BTC/USD",
	"ETH/USD",
	"SOL/USD",
	"XRP/USD",
	"ADA/USD",
	"DOGE/USD",
	"AVAX/USD",
	"DOT/USD",
	"LINK/USD",
	"LTC/USD",
];

export const rememberSymbol = (symbol: string) => {
	if (symbol.includes("/") && !SYMBOLS.includes(symbol)) {
		SYMBOLS.push(symbol);
	}
};

/*
The longest known symbol at a segment boundary, then the sentence tail.
*/
export const splitSymbol = (
	path: string,
): { symbol: string; rest: string } | null => {
	let best: string | null = null;

	for (const symbol of SYMBOLS) {
		if (
			(best === null || symbol.length > best.length) &&
			(path === symbol || path.startsWith(`${symbol}/`))
		) {
			best = symbol;
		}
	}

	if (best === null) return null;

	return { symbol: best, rest: path.slice(best.length + 1) };
};

export const COGNITION_ORIGIN = "cognition";
export const COGNITION_TREE_ROLE = "tree";
export const COGNITION_MATCH_ROLE = "match";
export const LAYER_COGNITION = "cognition";

export const TRAIN_REGIONS = "train/regions/";

export type CognitionBranch = {
	hash: string;
	depth: number;
	visits: number;
	meanEdge: number;
	confidence: number;
	policy: string;
	/* False when the branch was derived from a policy key, with no tree observation. */
	observed?: boolean;
};

export type CognitionGate = {
	tickEpoch: string;
	enabled: boolean;
	surpriseZ: number;
	warmup: string;
	learningRate: number;
};

export type CognitionProblem = {
	level: "error" | "warning";
	reason: string;
	fix: string;
};

export type CognitionTreeFrame = {
	epoch: string;
	paths: number;
	trading: boolean;
	gate: CognitionGate | null;
	keys: string[];
	branches: CognitionBranch[];
	problem: CognitionProblem | null;
};

export type CognitionMatch = {
	symbol: string;
	tick: number;
	tokens: string[];
	stored: boolean;
	match: string;
	depth: number;
	action: string;
	livePath?: string;
	triggerPath?: string;
	policyPath?: string;
	similarity?: number;
	exactMatch?: boolean;
	blockedReason?: string;
	/* Tokens the energy gate refused at this sentence's frontier. */
	rejected?: string[];
};

export type CognitionState = {
	epoch: string;
	policyKeys: string[];
	/** Threshold per explore key. Absent until a settled lot has written one. */
	thresholds: Record<string, number>;
	matches: Record<string, CognitionMatch>;
	problem: CognitionProblem | null;
	tree: CognitionTreeFrame | null;
};

export const cognitionStore = createStore<CognitionState>({
	epoch: "0",
	policyKeys: [],
	thresholds: {},
	matches: {},
	problem: null,
	tree: null,
});

const REGION_COUNT = 15;

type RegionRing = { tick: number; row: Array<number | undefined> };

const regionRings = new Map<string, RegionRing[]>();

export const regionHistoryStore = createStore<
	Record<string, Array<Array<number | undefined>>>
>({});

const noteRegions = (
	symbol: string,
	tick: number,
	tokens: string[],
	brightness: number[],
) => {
	if (tokens.length === 0) return;

	const prior = regionRings.get(symbol) ?? [];
	const last = prior[prior.length - 1];

	/*
	The new row starts from the previous row: a region this tick did not
	observe keeps its last brightness instead of going dark and reappearing
	two frames later.
	*/
	const row =
		last !== undefined
			? last.row.slice()
			: new Array<number | undefined>(REGION_COUNT).fill(undefined);

	tokens.forEach((token, index) => {
		const sector = Number(token.slice(1));
		const value = brightness[index];

		if (
			Number.isInteger(sector) &&
			sector >= 0 &&
			sector < REGION_COUNT &&
			value !== undefined
		) {
			row[sector] = value;
		}
	});

	/*
	The row for this tick is a replace, not a new ring entry: the region
	layers of one training frame arrive one by one.
	*/
	const next =
		last !== undefined && last.tick === tick
			? prior.slice(0, -1)
			: prior.slice(-23);
	next.push({ tick, row });
	regionRings.set(symbol, next);

	regionHistoryStore.setState((rows) => ({
		...rows,
		[symbol]: next.map((point) => point.row),
	}));
};

const REGION = /^R\d{2}$/;

export const parseTrainRegionsIndex = (
	index: string,
): { symbol: string; tokens: string[] } | null => {
	const start = index.indexOf(TRAIN_REGIONS);
	if (start < 0) return null;

	const tail = index.slice(start + TRAIN_REGIONS.length);
	const split = splitSymbol(tail);
	if (!split) return null;

	rememberSymbol(split.symbol);

	/*
	The tail is <symbol>/<sentence...>/<leaf>, where the leaf is the region
	(a per-brightness layer) or "tokens" (the live sentence, whose payload
	carries the token list).
	*/
	const parts = split.rest.split("/").filter(Boolean);
	const leaf = parts.pop();
	if (leaf === undefined) return null;

	if (leaf === "tokens") {
		return { symbol: split.symbol, tokens: [] };
	}

	if (!REGION.test(leaf)) return null;

	return { symbol: split.symbol, tokens: [...parts, leaf] };
};

/** S3 object key from a layer index (suffix after the frame prefix). */
export const s3KeyFromIndex = (
	framePrefix: string,
	index: string,
): string | null => {
	const root = `${framePrefix}/`;
	if (!index.startsWith(root)) return null;
	const key = index.slice(root.length);
	return key === "" ? null : key;
};

const policyKeyFromIndex = (
	index: string,
	framePrefix?: string,
): string | null => {
	if (framePrefix) {
		const key = s3KeyFromIndex(framePrefix, index);
		if (key) return key;
	}
	if (index.startsWith("model/explore/")) {
		return index;
	}
	const match = index.match(/model\/current\/(.+\/(?:enter|exit))\.bin/);
	if (match) return `model/current/${match[1]}.bin`;
	return null;
};

export const ingestTrainRegions = (
	tick: number,
	index: string,
	payload: unknown,
) => {
	const parsed = parseTrainRegionsIndex(index);
	if (!parsed) return false;

	/*
	Sentences arrive as one layer — a token list under /tokens. It feeds the
	match, never a ring.
	*/
	if (parsed.tokens.length === 0) {
		const tokens = Array.isArray(payload)
			? payload.filter((token): token is string => typeof token === "string")
			: [];

		if (tokens.length === 0) return true;

		cognitionStore.setState((state) => {
			const prior = state.matches[parsed.symbol];

			return {
				...state,
				matches: {
					...state.matches,
					[parsed.symbol]: {
						symbol: parsed.symbol,
						tick,
						tokens,
						stored: false,
						match: "",
						depth: tokens.length,
						action: "",
						rejected:
							prior != null && prior.tick === tick ? prior.rejected : [],
					},
				},
			};
		});

		return true;
	}

	/*
	Per-region layers carry brightness under a <R##> tail: the ring row for
	this tick. A payload that is not brightness belongs to no ring and does
	not clear what prior layers set.
	*/
	const brightness = Array.isArray(payload)
		? payload.map((value) => Number(value)).filter(Number.isFinite)
		: [];

	if (brightness.length > 0) {
		noteRegions(parsed.symbol, tick, parsed.tokens, brightness);
	}

	cognitionStore.setState((state) => {
		const prior = state.matches[parsed.symbol];
		const tokens = parsed.tokens;

		return {
			...state,
			matches: {
				...state.matches,
				[parsed.symbol]: {
					symbol: parsed.symbol,
					tick,
					tokens,
					stored: false,
					match: "",
					depth: tokens.length,
					action: "",
					rejected: prior != null && prior.tick === tick ? prior.rejected : [],
				},
			},
		};
	});

	return true;
};

export const branchesFromPolicyKeys = (keys: string[]): CognitionBranch[] => {
	const out: CognitionBranch[] = [];

	for (const raw of keys) {
		if (!raw.includes("enter.bin")) continue;

		const trimmed = raw
			.replace(/^model\/explore\//, "")
			.replace(/^policy\/current\//, "")
			.replace(/\/enter\.bin$/, "");
		const depth = trimmed.split("/").filter(Boolean).length;

		out.push({
			hash: trimmed,
			depth,
			visits: 0,
			meanEdge: 0,
			confidence: 0,
			policy: "ENTER",
			observed: false,
		});
	}

	return out;
};

const isBranch = (value: unknown): value is CognitionBranch => {
	const branch = value as Partial<CognitionBranch> | null;
	return (
		typeof branch?.hash === "string" &&
		typeof branch.depth === "number" &&
		typeof branch.visits === "number" &&
		typeof branch.meanEdge === "number" &&
		typeof branch.confidence === "number" &&
		typeof branch.policy === "string"
	);
};

const isGate = (value: unknown): value is CognitionGate | null => {
	if (value === null) return true;
	const gate = value as Partial<CognitionGate> | undefined;
	return (
		typeof gate?.tickEpoch === "string" &&
		typeof gate.enabled === "boolean" &&
		typeof gate.surpriseZ === "number" &&
		typeof gate.warmup === "string" &&
		typeof gate.learningRate === "number"
	);
};

const isProblem = (value: unknown): value is CognitionProblem | null => {
	if (value === null) return true;
	const problem = value as Partial<CognitionProblem> | undefined;
	return (
		(problem?.level === "error" || problem?.level === "warning") &&
		typeof problem.reason === "string" &&
		problem.reason !== "" &&
		typeof problem.fix === "string" &&
		problem.fix !== ""
	);
};

const isTree = (value: unknown): value is CognitionTreeFrame => {
	const tree = value as Partial<CognitionTreeFrame> | null;
	return (
		typeof tree?.epoch === "string" &&
		typeof tree.paths === "number" &&
		typeof tree.trading === "boolean" &&
		isGate(tree.gate ?? null) &&
		Array.isArray(tree.keys) &&
		tree.keys.every((key) => typeof key === "string") &&
		Array.isArray(tree.branches) &&
		tree.branches.every(isBranch) &&
		isProblem(tree.problem ?? null)
	);
};

const isMatch = (value: unknown): value is CognitionMatch => {
	const match = value as Partial<CognitionMatch> | null;
	return (
		typeof match?.symbol === "string" &&
		match.symbol !== "" &&
		typeof match.tick === "number" &&
		Array.isArray(match.tokens) &&
		match.tokens.every((token) => typeof token === "string") &&
		typeof match.stored === "boolean" &&
		typeof match.match === "string" &&
		typeof match.depth === "number" &&
		typeof match.action === "string"
	);
};

const layerText = (raw: unknown): string | null => {
	if (typeof raw === "string") {
		try {
			const decoded = atob(raw);
			if (decoded !== "") return decoded;
		} catch {
			// The layer is already text.
		}
		return raw;
	}

	if (
		Array.isArray(raw) &&
		raw.every((item) => typeof item === "number" && item >= 0 && item <= 255)
	) {
		return new TextDecoder().decode(new Uint8Array(raw));
	}

	return null;
};

const layerNumber = (raw: unknown): number | null => {
	if (typeof raw === "number" && Number.isFinite(raw)) return raw;

	const text = layerText(raw);
	if (text == null) return null;

	try {
		const parsed = JSON.parse(text);
		return typeof parsed === "number" && Number.isFinite(parsed) ? parsed : null;
	} catch {
		return null;
	}
};

/*
thresholdFromStored reads the path book off a stored explore value.
The desk starts the sum at 0.50 and each settled lot appends its step.
A value with no threshold layer has not been scored.
*/
export const thresholdFromStored = (payload: unknown): number | null => {
	if (!payload || typeof payload !== "object") return null;

	const body = payload as {
		Index?: unknown;
		Layers?: unknown;
		index?: unknown;
		layers?: unknown;
	};
	const names = Array.isArray(body.Index) ? body.Index : body.index;
	const layers = Array.isArray(body.Layers) ? body.Layers : body.layers;
	if (!Array.isArray(names) || !Array.isArray(layers)) return null;

	let sum = 0.5;
	let found = false;

	for (let i = 0; i < names.length; i++) {
		const name = names[i];
		if (typeof name !== "string" || !name.includes("threshold")) continue;

		const step = layerNumber(layers[i]);
		if (step === null) continue;

		sum += step;
		found = true;
	}

	return found ? sum : null;
};

/*
thresholdFromExploreBytes reads a path book off one S3 list layer: either a
JSON-marshaled explore value (hub ListPrefix) or a nested wire Value blob.
*/
export const thresholdFromExploreBytes = (
	bytes: Uint8Array,
): number | null => {
	const json = tryDecodeJsonBytes(bytes);
	const fromJSON = thresholdFromStored(json);
	if (fromJSON !== null) return fromJSON;

	const nested = parseOneValue(bytes);
	if (!nested) return null;

	let sum = 0.5;
	let found = false;

	for (let i = 0; i < nested.index.length; i++) {
		if (!nested.index[i].includes("threshold")) continue;

		const step = layerNumber(tryDecodeJsonBytes(nested.layers[i]));
		if (step === null) continue;

		sum += step;
		found = true;
	}

	return found ? sum : null;
};

const sameStringList = (left: string[], right: string[]): boolean =>
	left.length === right.length && left.every((key, index) => key === right[index]);

const sameThresholds = (
	left: Record<string, number>,
	right: Record<string, number>,
): boolean => {
	const leftKeys = Object.keys(left);
	const rightKeys = Object.keys(right);
	if (leftKeys.length !== rightKeys.length) return false;
	for (const key of leftKeys) {
		if (left[key] !== right[key]) return false;
	}
	return true;
};

/*
ingestS3Prefix applies a hub ListPrefix frame. Scope is the listed prefix;
each layer is the stored object (a data.Value on S3). Keys and fields come
from Value.readings() — no extra backend publish path.
*/
export const ingestS3Prefix = (value: Value) => {
	if (value.origin !== "s3" || value.role !== "prefix") {
		return false;
	}

	if (value.scope !== "model/explore") {
		return true;
	}

	const policyKeys = value.index
		.map((index) => s3KeyFromIndex(value.prefix, index))
		.filter((key): key is string => key != null)
		.sort();

	const thresholds: Record<string, number> = {};
	for (let i = 0; i < value.index.length; i++) {
		const key = s3KeyFromIndex(value.prefix, value.index[i]);
		if (key === null || !key.startsWith("model/explore/")) continue;

		const threshold = thresholdFromExploreBytes(value.layers[i]);
		if (threshold === null) continue;
		thresholds[key] = threshold;
	}

	const epoch =
		value.epoch != null && value.epoch !== 0n ? String(value.epoch) : null;

	cognitionStore.setState((state) => {
		const nextEpoch = epoch ?? state.epoch;
		if (
			nextEpoch === state.epoch &&
			sameStringList(state.policyKeys, policyKeys) &&
			sameThresholds(state.thresholds, thresholds)
		) {
			return state;
		}

		return {
			...state,
			epoch: nextEpoch,
			policyKeys,
			thresholds,
		};
	});
	return true;
};

export const ingestPolicyKeys = (keys: unknown, epoch?: string) => {
	if (!Array.isArray(keys)) return false;

	const policyKeys: string[] = [];
	for (const key of keys) {
		if (typeof key !== "string" || key === "") continue;
		policyKeys.push(key);
	}

	cognitionStore.setState((state) => ({
		...state,
		epoch: epoch && epoch !== "" ? epoch : state.epoch,
		policyKeys,
	}));
	return true;
};

type PathWalk = {
	symbol?: string;
	phase?: string;
	live?: string;
	trigger?: string;
	policy?: string;
	enter?: boolean;
	outcome?: string;
};

/*
ingestPathWalks applies one tick's changed sentences. Symbols whose letter
did not change are absent. An exit in the batch refreshes the stored trie.
*/
export const ingestPathWalks = (tick: number, payload: unknown): boolean => {
	if (!Array.isArray(payload)) return false;

	const rows = payload.filter(
		(row): row is PathWalk =>
			!!row &&
			typeof row === "object" &&
			typeof (row as PathWalk).symbol === "string" &&
			(row as PathWalk).symbol !== "",
	);

	if (rows.length === 0) return false;

	const exited = rows.some((row) => row.outcome === "exit submitted");

	cognitionStore.setState((state) => {
		const matches = { ...state.matches };

		for (const row of rows) {
			const symbol = row.symbol as string;
			const phase = row.phase || row.policy || row.trigger || row.live || "";
			const tokens = phase.split("/").filter(Boolean);
			const prior = matches[symbol];

			matches[symbol] = {
				symbol,
				tick,
				tokens,
				stored: prior?.stored ?? false,
				match: prior?.match ?? "",
				depth: tokens.length,
				action: row.enter ? "enter" : "exit",
				livePath: row.live,
				triggerPath: row.trigger,
				policyPath: row.policy || phase,
				rejected: prior?.tick === tick ? prior.rejected : (prior?.rejected ?? []),
			};
		}

		return { ...state, matches };
	});

	if (exited) {
		trySendCommand({ type: "s3", prefix: "model/explore" });
	}

	return true;
};

export const ingestCognitionFrame = (role: string, payload: unknown) => {
	if (role === "keys" && payload && typeof payload === "object") {
		const body = payload as { keys?: unknown; epoch?: unknown };
		return ingestPolicyKeys(
			body.keys,
			typeof body.epoch === "string" ? body.epoch : undefined,
		);
	}

	if (role === COGNITION_TREE_ROLE && isTree(payload)) {
		const tree = payload as CognitionTreeFrame;
		cognitionStore.setState(() => ({
			epoch: tree.epoch,
			policyKeys: [...tree.keys],
			thresholds: {},
			matches: {},
			problem: tree.problem ?? null,
			tree: { ...tree, gate: tree.gate ?? null, problem: tree.problem ?? null },
		}));
		return true;
	}

	if (role === COGNITION_MATCH_ROLE && isMatch(payload)) {
		cognitionStore.setState((state) => ({
			...state,
			matches: { ...state.matches, [payload.symbol]: payload },
		}));
		return true;
	}

	console.error(
		"ingestCognitionFrame: malformed cognition frame",
		role,
		payload,
	);
	return false;
};

export const ingestPolicyLayers = (
	epoch: bigint,
	layers: Array<{ index: string; bytes: Uint8Array }>,
) => {
	const keys: string[] = [];

	for (const layer of layers) {
		const key = policyKeyFromIndex(layer.index);
		if (key && !keys.includes(key)) keys.push(key);
	}

	if (keys.length === 0) return;

	cognitionStore.setState((state) => ({
		...state,
		epoch: String(epoch),
		policyKeys: keys,
	}));
};

const REJECT = "train/reject/";

export const parseTrainRejectIndex = (
	index: string,
): { symbol: string; token: string } | null => {
	const start = index.indexOf(REJECT);
	if (start < 0) return null;

	const parts = index
		.slice(start + REJECT.length)
		.split("/")
		.filter(Boolean);
	const token = parts.at(-1);
	if (!token || !REGION.test(token)) return null;

	const symbol = parts.slice(0, -1).join("/");
	if (!symbol) return null;

	return { symbol, token };
};

export const ingestTrainReject = (tick: number, index: string) => {
	const parsed = parseTrainRejectIndex(index);
	if (!parsed) return false;

	cognitionStore.setState((state) => {
		const prior = state.matches[parsed.symbol];
		const sameTick = prior != null && prior.tick === tick;
		const rejected = sameTick
			? [...(prior.rejected ?? []), parsed.token]
			: [parsed.token];
		const base = prior ?? {
			symbol: parsed.symbol,
			tick,
			tokens: [],
			stored: false,
			match: "",
			depth: 0,
			action: "",
		};

		return {
			...state,
			matches: {
				...state.matches,
				[parsed.symbol]: { ...base, tick, rejected },
			},
		};
	});

	return true;
};
