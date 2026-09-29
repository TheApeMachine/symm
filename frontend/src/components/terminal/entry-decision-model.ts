import type { DecisionT } from "#/providers/telemetry/telemetry/decision";
import type { PositionT } from "#/providers/telemetry/telemetry/position";

export type DecisionEvidence = {
	key: string;
	value: number;
};

export type FrozenEntryDecision = {
	id: string;
	action: string;
	symbol: string;
	atNs: bigint;
	cause: string;
	reason: string;
	opportunity: boolean;
	opportunityType: string;
	opportunityPhase: string;
	predictiveReady: boolean;
	predictiveStatus: string;
	confidence: number;
	direction: number;
	forecastSource: string;
	forecastModel: string;
	forecastHorizon: bigint;
	calibrationCount: bigint;
	allocationClass: string;
	proposedNotional: string;
	proposedQuantity: string;
	referencePrice: string;
	availableCapital: string;
	openPositions: bigint;
	entryCost: {
		entryPrice: string;
		bestAsk: string;
		bestBid: string;
		midpoint: string;
		grossNotional: string;
		entryFee: string;
		roundTripFees: string;
		spread: string;
		impact: string;
		breakEven: string;
	};
	evidence: DecisionEvidence[];
};

const text = (value: string | null): string => value ?? "";

export const findDecision = (
	rows: PositionT[],
	symbol: string,
): DecisionT | null => {
	for (const row of rows) {
		if (row?.holding?.symbol === symbol) {
			return row.decision ?? null;
		}
	}

	return null;
};

/*
readEntryDecision copies the position's retained entry decision into ordinary
values. No FlatBuffer view escapes this call, so later position ticks cannot
mutate the frozen snapshot displayed by the modal.
*/
export const readEntryDecision = (
	rows: PositionT[],
	symbol: string,
): FrozenEntryDecision | null => {
	const decision = findDecision(rows, symbol);

	if (decision === null) {
		return null;
	}

	// Recovery can reconstruct an exposed lot from venue balances without the
	// historical arbitration. An object-shaped placeholder is not an entry
	// snapshot: only a retained enter decision is truthful enough to display.
	if (decision.action !== "enter") {
		return null;
	}

	const cost = decision.entryCost;
	const evidence: DecisionEvidence[] = [];

	for (const entry of decision.alternatives ?? []) {
		if (entry?.name) {
			evidence.push({ key: String(entry.name), value: entry.value });
		}
	}

	evidence.sort((left, right) => left.key.localeCompare(right.key));

	return {
		id: text(decision.id as string | null),
		action: text(decision.action as string | null),
		symbol: text(decision.symbol as string | null),
		atNs: decision.at,
		cause: text(decision.cause as string | null),
		reason: text(decision.reason as string | null).replace(/^planner:\s*/, ""),
		opportunity: decision.opportunity,
		opportunityType: text(decision.opportunityType as string | null),
		opportunityPhase: text(decision.opportunityPhase as string | null),
		predictiveReady: decision.predictiveReady,
		predictiveStatus: text(decision.predictiveStatus as string | null),
		confidence: decision.confidence,
		direction: decision.direction,
		forecastSource: text(decision.forecastSource as string | null),
		forecastModel: text(decision.forecastModel as string | null),
		forecastHorizon: decision.forecastHorizon,
		calibrationCount: decision.calibrationCount,
		allocationClass: text(decision.allocationClass as string | null),
		proposedNotional: text(decision.proposedNotional as string | null),
		proposedQuantity: text(decision.proposedQuantity as string | null),
		referencePrice: text(decision.referencePrice as string | null),
		availableCapital: text(decision.availableCapital as string | null),
		openPositions: decision.openPositions,
		entryCost: {
			entryPrice: text(cost?.entryPrice as string | null),
			bestAsk: text(cost?.bestAsk as string | null),
			bestBid: text(cost?.bestBid as string | null),
			midpoint: text(cost?.midpoint as string | null),
			grossNotional: text(cost?.grossNotional as string | null),
			entryFee: text(cost?.entryFee as string | null),
			roundTripFees: text(cost?.roundTripFees as string | null),
			spread: text(cost?.spread as string | null),
			impact: text(cost?.impact as string | null),
			breakEven: text(cost?.breakEven as string | null),
		},
		evidence,
	};
};

export const evidenceMeaning = (key: string): string => {
	switch (key) {
		case "probability:up":
			return "Estimated chance that price moves upward over the adaptive forecast horizon.";
		case "probability:profitable":
			return "Estimated chance that the move clears the complete entry and exit cost boundary.";
		case "return:expected_log":
			return "The center of the predicted return distribution. It had to beat break-even before entry.";
		case "return:break_even_log":
			return "The minimum return needed to recover fees, spread, and expected market impact.";
		case "return:scale":
			return "How widely outcomes were spread. Wider means the forecast was less tightly concentrated.";
		case "return:degrees_of_freedom":
			return "How heavily the forecast allowed for unusually large moves in either direction.";
		case "execution:coverage":
		case "execution:visible_coverage":
			return "How much of the requested quantity the visible order book could actually supply.";
		case "execution:spread":
		case "execution:spread_fraction":
			return "The fraction of entry price lost to the visible bid/ask gap.";
		case "execution:impact":
		case "execution:impact_fraction":
			return "The fraction of entry price lost by consuming multiple ask levels.";
		case "execution:friction":
		case "execution:friction_fraction":
			return "Spread and order-book impact combined—the immediate market cost before fees.";
		case "horizon:ticker_steps":
			return "How many future ticker observations the adaptive forecast covered.";
		case "features:directional":
			return "Count of usable inputs describing likely move direction.";
		case "features:estimability":
			return "Count of inputs describing whether the forecast was statistically usable.";
		case "features:execution_context":
			return "Count of inputs describing whether the move remained tradable after market costs.";
		case "features:semantic_review":
			return "Count of inputs that passed their declared meaning and routing constraints.";
		case "consensus:dominant":
			return "The dominant qualitative market move synthesized by the War Room council.";
		case "consensus:participants":
			return "Number of distinct advisors that deliberated on this decision.";
		case "consensus:synergies":
			return "Number of reinforcing cross-advisor synergy rules triggered.";
		case "consensus:vetoes":
			return "Number of cross-advisor veto rules that suppressed conflicting moves.";
		case "branch:enter:blended":
			return "Blended economic value for entering: real rollout outcomes merged with counterfactual evidence.";
		case "branch:enter:mean":
			return "Sample average net-wealth change from rollouts that took the enter action.";
		case "branch:enter:visits":
			return "Number of simulated rollout paths that explored the enter action.";
		case "branch:enter:counterfactual_mass":
			return "Virtual experience mass accrued on the enter branch via Pearl counterfactuals.";
		case "branch:wait:blended":
			return "Blended economic value for waiting / doing nothing.";
		case "branch:wait:mean":
			return "Sample average net-wealth change from rollouts that took the wait action.";
		case "branch:wait:visits":
			return "Number of simulated rollout paths that explored waiting.";
		case "branch:wait:counterfactual_mass":
			return "Virtual experience mass accrued on the wait branch via Pearl counterfactuals.";
		case "search:expected_outcome":
			return "Expected dollar outcome modeled over the search horizon for the selected action.";
		case "search:outcome_uncertainty":
			return "Standard error of the simulated economic outcome across rollouts.";
		case "search:visits":
			return "Total MCTS rollout iterations completed across all branches.";
		default:
			if (key.startsWith("move:")) {
				return `Probability mass assigned by the advisor council to ${key.slice(5).replace(/_/g, " ")}.`;
			}
			return "A named fact recorded on the frozen entry decision.";
	}
};

export const evidenceValue = (evidence: DecisionEvidence): string => {
	if (
		evidence.key.startsWith("probability:") ||
		evidence.key.startsWith("execution:") ||
		evidence.key.startsWith("move:")
	) {
		return `${(evidence.value * 100).toFixed(2)}%`;
	}

	if (evidence.key === "consensus:dominant") {
		const moves: Record<number, string> = {
			3: "explosive_pump",
			2: "steady_trend",
			1: "weak_drift",
			0: "stagnant",
			[-1]: "weak_bleed",
			[-2]: "structural_pullback",
			[-3]: "flash_dump",
		};
		return moves[Math.round(evidence.value)] ?? evidence.value.toFixed(0);
	}

	if (
		evidence.key.startsWith("features:") ||
		evidence.key.endsWith(":visits") ||
		evidence.key === "search:visits" ||
		evidence.key.startsWith("consensus:") ||
		evidence.key === "horizon:ticker_steps"
	) {
		return evidence.value.toFixed(0);
	}

	if (evidence.key.startsWith("return:") && evidence.key.endsWith("_log")) {
		return `${evidence.value.toFixed(6)} log · ${(Math.expm1(evidence.value) * 100).toFixed(2)}% equivalent`;
	}

	return evidence.value.toFixed(6);
};
