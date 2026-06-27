import type { WalkStep, WalkTrace } from "#/collections/playbook";
import { entryLineStats, fixed, whyLabel } from "#/components/terminal/decision-format";
import type {
	TerminalDecisionRow,
	TerminalKernel,
} from "#/components/terminal/model";

type KernelsForWalk = TerminalKernel[] | ((symbol: string) => TerminalKernel[]);

export const walkVerdict = (
	steps: WalkStep[],
): TerminalDecisionRow["verdict"] => {
	if (steps.some((step) => step.outcome === "action")) {
		return "allow";
	}

	if (
		steps.some(
			(step) => step.outcome === "matched" || step.outcome === "parked",
		)
	) {
		return "in-play";
	}

	if (steps.some((step) => step.outcome === "rejected")) {
		return "blocked";
	}

	return "blocked";
};

const walkWhy = (steps: WalkStep[]): string => {
	const lastReason = [...steps]
		.reverse()
		.find(
			(step) => typeof step.reason === "string" && step.reason.trim() !== "",
		);

	return whyLabel(lastReason?.reason);
};

const rankedSignals = (kernels: TerminalKernel[]) =>
	kernels
		.filter(
			(kernel) => kernel.confidencePercent > 0 || kernel.surprisePercent > 0,
		)
		.sort((left, right) => {
			const leftScore = Math.max(
				left.confidencePercent,
				left.surprisePercent * 0.75,
			);
			const rightScore = Math.max(
				right.confidencePercent,
				right.surprisePercent * 0.75,
			);

			return rightScore - leftScore;
		})
		.slice(0, 5)
		.map((kernel) => ({
			source: kernel.source,
			confidence: Math.max(
				kernel.confidencePercent / 100,
				kernel.surprisePercent / 100,
			),
		}));

export const combinedScoreFromKernels = (kernels: TerminalKernel[]): number => {
	const signals = rankedSignals(kernels);

	if (signals.length === 0) {
		return 0;
	}

	return (
		signals.reduce((sum, signal) => sum + signal.confidence, 0) / signals.length
	);
};

export const decisionRowFromWalk = (
	walkTrace: WalkTrace,
	kernels: TerminalKernel[],
	entryLine: number,
): TerminalDecisionRow => {
	const signals = rankedSignals(kernels);
	const depth = walkTrace.active_path?.length ?? 0;
	const matchedSteps = walkTrace.steps.filter(
		(step) => step.outcome === "matched" || step.outcome === "parked",
	).length;
	const scoreValue = Math.min(
		1,
		combinedScoreFromKernels(kernels) + depth * 0.08 + matchedSteps * 0.04,
	);
	const edge = scoreValue - entryLine;
	const edgePositive = edge > 0;

	return {
		key: `${walkTrace.symbol}:walk`,
		symbol: walkTrace.symbol,
		source: signals[0]?.source ?? "walk",
		scoreText: fixed(scoreValue),
		scoreValue,
		verdict: walkVerdict(walkTrace.steps),
		why: walkWhy(walkTrace.steps),
		signals,
		edgeText: `${edgePositive ? "+" : "−"}${fixed(Math.abs(edge))} / ${fixed(Math.abs(entryLine))}`,
		edgePositive,
	};
};

export const mergeTerminalDecisionRows = (
	walkRows: TerminalDecisionRow[],
	traceRows: TerminalDecisionRow[],
): TerminalDecisionRow[] => {
	const bySymbol = new Map<string, TerminalDecisionRow>();

	for (const row of traceRows) {
		bySymbol.set(row.symbol, row);
	}

	for (const row of walkRows) {
		const existing = bySymbol.get(row.symbol);

		if (existing === undefined) {
			bySymbol.set(row.symbol, row);

			continue;
		}

		bySymbol.set(row.symbol, {
			...row,
			...existing,
			scoreText: existing.scoreText,
			scoreValue: existing.scoreValue,
			signals: existing.signals.length > 0 ? existing.signals : row.signals,
			source: existing.source !== "decision" ? existing.source : row.source,
			verdict: row.verdict,
			why: row.why !== "—" ? row.why : existing.why,
			key: existing.key,
			edgeText: existing.edgeText,
			edgePositive: existing.edgePositive,
		});
	}

	return [...bySymbol.values()]
		.sort((left, right) => right.scoreValue - left.scoreValue)
		.slice(0, 16);
};

export const terminalDecisionsFromWalk = (
	walkEvaluations: Record<string, WalkTrace>,
	kernels: KernelsForWalk,
): TerminalDecisionRow[] => {
	const kernelsForSymbol =
		typeof kernels === "function" ? kernels : () => kernels;
	const provisional = Object.values(walkEvaluations).map((walkTrace) => {
		const scoreValue = combinedScoreFromKernels(kernelsForSymbol(walkTrace.symbol));

		return { walkTrace, scoreValue };
	});
	const scores = provisional.map((entry) => entry.scoreValue);
	const { line: entryLine } = entryLineStats(scores);

	return provisional
		.map(({ walkTrace }) =>
			decisionRowFromWalk(walkTrace, kernelsForSymbol(walkTrace.symbol), entryLine),
		)
		.sort((left, right) => right.scoreValue - left.scoreValue)
		.slice(0, 16);
};
