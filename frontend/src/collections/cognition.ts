import { createStore } from "@tanstack/store";
import { triePrefix } from "#/collections/book";
import { filterExploreKeys } from "#/components/learning/explore-path";

export type CognitionProblem = {
	level: "error" | "warning";
	reason: string;
	fix: string;
};

export type CognitionGate = {
	tickEpoch: string;
	enabled: boolean;
	surpriseZ: number;
	warmup: string;
	learningRate: number;
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
	rejected?: string[];
};

export type CognitionState = {
	epoch: string;
	policyKeys: string[];
	thresholds: Record<string, number>;
	matches: Record<string, CognitionMatch>;
	problem: CognitionProblem | null;
	tree: {
		trading?: boolean;
		gate?: CognitionGate | null;
		problem?: CognitionProblem | null;
	} | null;
};

export const cognitionStore = createStore<CognitionState>({
	epoch: "0",
	policyKeys: [],
	thresholds: {},
	matches: {},
	problem: null,
	tree: null,
});

const sameStringList = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((key, index) => key === right[index]);

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

/** Applies a hub `pathbook` listing (model/paths trie keys + thresholds). */
export const ingestExploreListPrefix = (input: {
	scope: string;
	epoch: string | null;
	policyKeys: string[];
	thresholds: Record<string, number>;
}) => {
	if (input.scope !== triePrefix()) {
		return;
	}

	cognitionStore.setState((state) => {
		const nextEpoch = input.epoch ?? state.epoch;
		const nextThresholds = { ...state.thresholds, ...input.thresholds };
		const merged = new Set(state.policyKeys);
		for (const key of input.policyKeys) merged.add(key);
		const nextKeys = filterExploreKeys([...merged].sort(), nextThresholds);

		if (
			nextEpoch === state.epoch &&
			sameStringList(state.policyKeys, nextKeys) &&
			sameThresholds(state.thresholds, nextThresholds)
		) {
			return state;
		}

		return {
			...state,
			epoch: nextEpoch,
			policyKeys: nextKeys,
			thresholds: nextThresholds,
		};
	});
};