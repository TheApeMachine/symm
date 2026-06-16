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

export const parseWalkTrace = (
	raw: Record<string, unknown>,
): WalkTrace | null => {
	if (typeof raw.symbol !== "string" || raw.symbol === "") {
		return null;
	}

	if (!Array.isArray(raw.steps) || !raw.steps.every(isWalkStep)) {
		return null;
	}

	const activePath = raw.active_path;

	if (
		activePath !== undefined &&
		(!Array.isArray(activePath) || !activePath.every(Number.isInteger))
	) {
		return null;
	}

	return {
		symbol: raw.symbol,
		steps: raw.steps,
		active_path: activePath,
	};
};

export const playbookStore = createStore(
	{
		branches: [] as PlaybookBranch[],
		walkTrace: null as WalkTrace | null,
	},
	({ setState }) => ({
		updateBranches: (branches: PlaybookBranch[]) =>
			setState((prev) => ({
				...prev,
				branches: branches,
			})),
		updateWalkTrace: (walkTrace: WalkTrace | null) =>
			setState((prev) => ({
				...prev,
				walkTrace: walkTrace,
			})),
	}),
);
