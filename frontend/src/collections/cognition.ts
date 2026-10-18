import { createStore } from "@tanstack/store";

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
	/* Tokens the energy gate refused at this sentence's frontier. */
	rejected?: string[];
};

export type CognitionState = {
	epoch: string;
	policyKeys: string[];
	matches: Record<string, CognitionMatch>;
	problem: CognitionProblem | null;
	tree: CognitionTreeFrame | null;
};

export const cognitionStore = createStore<CognitionState>({
	epoch: "0",
	policyKeys: [],
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
	const row =
		last !== undefined && last.tick === tick
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
	const parts = tail.split("/").filter(Boolean);
	if (parts.length === 0) return null;

	const tokens: string[] = [];
	let symbolEnd = 0;

	for (let i = parts.length - 1; i >= 0; i--) {
		if (REGION.test(parts[i])) {
			tokens.unshift(parts[i]);
			symbolEnd = i;
			continue;
		}
		break;
	}

	if (tokens.length === 0) return null;

	const symbol = parts.slice(0, symbolEnd).join("/");
	if (!symbol) return null;

	return { symbol, tokens };
};

const policyKeyFromIndex = (index: string): string | null => {
	const match = index.match(/policy\/current\/(.+\/(?:enter|exit))\.bin/);
	if (!match) return null;
	return `policy/current/${match[1]}.bin`;
};

export const ingestPolicyKey = (key: string, epoch?: string) => {
	const normalized = key.includes("policy/current/")
		? key
		: `policy/current/${key.replace(/^\//, "")}`;

	cognitionStore.setState((state) => {
		const nextEpoch = epoch ?? state.epoch;
		const epochChanged = epoch !== undefined && epoch !== state.epoch;
		const policyKeys = epochChanged
			? [normalized]
			: state.policyKeys.includes(normalized)
				? state.policyKeys
				: [...state.policyKeys, normalized];

		return {
			...state,
			epoch: nextEpoch,
			policyKeys,
			tree: epochChanged ? null : state.tree,
		};
	});
};

export const ingestTrainRegionsLayer = (
	tick: number,
	index: string,
	brightness: number[] = [],
) => {
	const parsed = parseTrainRegionsIndex(index);
	if (!parsed) return false;

	noteRegions(parsed.symbol, tick, parsed.tokens, brightness);

	cognitionStore.setState((state) => {
		const prior = state.matches[parsed.symbol];
		const tokens = parsed.tokens;
		const path = tokens.join("/");
		const aligned = prior != null && prior.match !== "" && prior.match === path;

		return {
			...state,
			matches: {
				...state.matches,
				[parsed.symbol]: {
					symbol: parsed.symbol,
					tick,
					tokens,
					stored: aligned ? prior.stored : false,
					match: aligned ? prior.match : "",
					depth: tokens.length,
					action: aligned ? prior.action : "",
					rejected: prior != null && prior.tick === tick ? prior.rejected : [],
				},
			},
		};
	});

	return true;
};

export const branchesFromPolicyKeys = (
	keys: string[],
): CognitionBranch[] => {
	const out: CognitionBranch[] = [];

	for (const raw of keys) {
		if (!raw.includes("enter.bin")) continue;

		const trimmed = raw.replace(/^policy\/current\//, "").replace(/\/enter\.bin$/, "");
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

export const ingestCognitionFrame = (role: string, payload: unknown) => {
	if (role === COGNITION_TREE_ROLE && isTree(payload)) {
		const tree = payload as CognitionTreeFrame;
		cognitionStore.setState(() => ({
			epoch: tree.epoch,
			policyKeys: [...tree.keys],
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
