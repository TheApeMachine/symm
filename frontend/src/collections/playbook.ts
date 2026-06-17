import { createStore } from "@tanstack/react-store";

export type PlaybookBranch = {
	condition_group?: {
		boolean?: string;
		conditions?: Array<{
			type?: string;
			left?: Record<string, unknown>;
			right?: Record<string, unknown>;
		}>;
	};
	action?: {
		type?: string;
		side?: string;
		fraction?: number;
	};
	branches?: PlaybookBranch[];
};

export type WalkStepOutcome = "rejected" | "matched" | "parked" | "action";

export type WalkStep = {
	path: number[];
	outcome: WalkStepOutcome;
	reason?: string;
};

export type WalkTrace = {
	symbol: string;
	steps: WalkStep[];
	active_path?: number[];
};

export const walkPathKey = (path: number[]): string => path.join(".");

const OUTCOME_RANK: Record<WalkStepOutcome, number> = {
	rejected: 0,
	parked: 1,
	matched: 2,
	action: 3,
};

const mergeWalkOutcome = (
	current: WalkStepOutcome | undefined,
	incoming: WalkStepOutcome,
): WalkStepOutcome => {
	if (current === undefined) {
		return incoming;
	}

	return OUTCOME_RANK[incoming] > OUTCOME_RANK[current] ? incoming : current;
};

/*
mergeWalkActivation keeps nodes that ever matched on the walk path lit green.
*/
export const mergeWalkActivation = (
	activated: Record<string, WalkStepOutcome>,
	walkTrace: WalkTrace,
): Record<string, WalkStepOutcome> => {
	const next = { ...activated };

	for (const step of walkTrace.steps) {
		if (step.outcome === "rejected") {
			continue;
		}

		const key = walkPathKey(step.path);
		next[key] = mergeWalkOutcome(next[key], step.outcome);
	}

	if (walkTrace.active_path) {
		for (let depth = 1; depth <= walkTrace.active_path.length; depth += 1) {
			const prefix = walkTrace.active_path.slice(0, depth);
			const key = walkPathKey(prefix);
			next[key] = mergeWalkOutcome(next[key], "matched");
		}
	}

	return next;
};

export type WalkNodeVisualState = "matched" | "action" | "rejected" | "idle";

/*
persistedNodeState maps walk history to stable node colors for the decision tree.
Activated branches stay green so a full descent remains visible across ticks.
*/
export const persistedNodeState = (
	path: number[],
	walkTrace: WalkTrace | null,
	activated: Record<string, WalkStepOutcome>,
): WalkNodeVisualState => {
	const key = walkPathKey(path);
	const persisted = activated[key];
	const step = walkStepForPath(walkTrace, path);
	const active = pathIsActive(walkTrace, path);

	if (persisted === "action" || step?.outcome === "action") {
		return "action";
	}

	if (
		persisted === "matched" ||
		persisted === "parked" ||
		step?.outcome === "matched" ||
		step?.outcome === "parked" ||
		active
	) {
		return "matched";
	}

	if (step?.outcome === "rejected") {
		return "rejected";
	}

	return "idle";
};

const pathsEqual = (left: number[] | undefined, right: number[]): boolean => {
	if (!left || left.length !== right.length) {
		return false;
	}

	return left.every((value, index) => value === right[index]);
};

export const walkStepForPath = (
	trace: WalkTrace | null,
	path: number[],
): WalkStep | null => {
	if (!trace) {
		return null;
	}

	return trace.steps.find((step) => pathsEqual(step.path, path)) ?? null;
};

export const pathIsActive = (
	trace: WalkTrace | null,
	path: number[],
): boolean => {
	if (!trace?.active_path) {
		return false;
	}

	return pathsEqual(trace.active_path, path);
};

const isWalkStepOutcome = (value: unknown): value is WalkStepOutcome =>
	value === "rejected" ||
	value === "matched" ||
	value === "parked" ||
	value === "action";

const isWalkStep = (value: unknown): value is WalkStep => {
	if (typeof value !== "object" || value === null) {
		return false;
	}

	const step = value as Record<string, unknown>;

	if (!Array.isArray(step.path)) {
		return false;
	}

	if (
		!step.path.every(
			(element): element is number =>
				typeof element === "number" && Number.isInteger(element),
		)
	) {
		return false;
	}

	return isWalkStepOutcome(step.outcome);
};

export const parseWalkTrace = (raw: unknown): WalkTrace | null => {
	if (typeof raw !== "object" || raw === null) {
		return null;
	}

	const frame = raw as Record<string, unknown>;

	if (typeof frame.symbol !== "string" || frame.symbol === "") {
		return null;
	}

	if (!Array.isArray(frame.steps) || !frame.steps.every(isWalkStep)) {
		return null;
	}

	const activePath = frame.active_path;

	if (
		activePath !== undefined &&
		(!Array.isArray(activePath) || !activePath.every(Number.isInteger))
	) {
		return null;
	}

	return {
		symbol: frame.symbol,
		steps: frame.steps,
		active_path: activePath,
	};
};

export const playbookStore = createStore(
	{
		branches: [] as PlaybookBranch[],
		walkTrace: null as WalkTrace | null,
		activatedPathOutcomes: {} as Record<string, WalkStepOutcome>,
	},
	({ setState }) => ({
		updateBranches: (branches: PlaybookBranch[]) =>
			setState((prev) => ({
				...prev,
				branches: branches,
				activatedPathOutcomes: {},
			})),
		updateWalkTrace: (walkTrace: WalkTrace | null) =>
			setState((prev) => {
				if (walkTrace === null) {
					return {
						...prev,
						walkTrace: null,
					};
				}

				return {
					...prev,
					walkTrace: walkTrace,
					activatedPathOutcomes: mergeWalkActivation(
						prev.activatedPathOutcomes,
						walkTrace,
					),
				};
			}),
	}),
);
