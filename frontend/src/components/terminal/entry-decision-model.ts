import type { positionStore } from "#/collections/app";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { EntryCost } from "#/providers/telemetry/telemetry/entry-cost";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";
import { Position } from "#/providers/telemetry/telemetry/position";
import { RiskPlan } from "#/providers/telemetry/telemetry/risk-plan";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

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
	risk: {
		present: boolean;
		entryNoiseBand: string;
		riskDistance: string;
		trailDistance: string;
		minEdge: string;
		maxLoss: string;
	};
	stopFloor: string;
	evidence: DecisionEvidence[];
};

type PositionState = ReturnType<typeof positionStore.get>;

const text = (value: string | null): string => value ?? "";

const findDecision = (
	state: PositionState,
	symbol: string,
): Decision | null => {
	const position = new Position();
	const holding = new Holding();
	const decision = new Decision();
	const frames = state.toArray();

	for (let frameIndex = frames.length - 1; frameIndex >= 0; frameIndex--) {
		const frame = frames[frameIndex];

		for (let rowIndex = 0; rowIndex < frame.rowsLength(); rowIndex++) {
			const row = frame.rows(rowIndex, position);
			const rowHolding = row?.holding(holding);

			if (rowHolding?.symbol() !== symbol) {
				continue;
			}

			return row?.decision(decision) ?? null;
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
	state: PositionState,
	symbol: string,
): FrozenEntryDecision | null => {
	const decision = findDecision(state, symbol);

	if (decision === null) {
		return null;
	}

	// Recovery can reconstruct an exposed lot from venue balances without the
	// historical arbitration. An object-shaped placeholder is not an entry
	// snapshot: only a retained enter decision is truthful enough to display.
	if (decision.action() !== "enter") {
		return null;
	}

	const cost = decision.entryCost(new EntryCost());
	const risk = decision.risk(new RiskPlan());
	const stop = decision.stoploss(new Stoploss());
	const evidence: DecisionEvidence[] = [];
	const alternative = new NamedNumber();

	for (let index = 0; index < decision.alternativesLength(); index++) {
		const entry = decision.alternatives(index, alternative);

		if (entry?.name()) {
			evidence.push({ key: entry.name() as string, value: entry.value() });
		}
	}

	evidence.sort((left, right) => left.key.localeCompare(right.key));

	return {
		id: text(decision.id()),
		action: text(decision.action()),
		symbol: text(decision.symbol()),
		atNs: decision.at(),
		cause: text(decision.cause()),
		reason: text(decision.reason()).replace(/^planner:\s*/, ""),
		opportunity: decision.opportunity(),
		opportunityType: text(decision.opportunityType()),
		opportunityPhase: text(decision.opportunityPhase()),
		predictiveReady: decision.predictiveReady(),
		predictiveStatus: text(decision.predictiveStatus()),
		confidence: decision.confidence(),
		direction: decision.direction(),
		forecastSource: text(decision.forecastSource()),
		forecastModel: text(decision.forecastModel()),
		forecastHorizon: decision.forecastHorizon(),
		calibrationCount: decision.calibrationCount(),
		allocationClass: text(decision.allocationClass()),
		proposedNotional: text(decision.proposedNotional()),
		proposedQuantity: text(decision.proposedQuantity()),
		referencePrice: text(decision.referencePrice()),
		availableCapital: text(decision.availableCapital()),
		openPositions: decision.openPositions(),
		entryCost: {
			entryPrice: text(cost?.entryPrice() ?? null),
			bestAsk: text(cost?.bestAsk() ?? null),
			bestBid: text(cost?.bestBid() ?? null),
			midpoint: text(cost?.midpoint() ?? null),
			grossNotional: text(cost?.grossNotional() ?? null),
			entryFee: text(cost?.entryFee() ?? null),
			roundTripFees: text(cost?.roundTripFees() ?? null),
			spread: text(cost?.spread() ?? null),
			impact: text(cost?.impact() ?? null),
			breakEven: text(cost?.breakEven() ?? null),
		},
		risk: {
			present: risk?.present() ?? false,
			entryNoiseBand: text(risk?.entryNoiseBand() ?? null),
			riskDistance: text(risk?.riskDistance() ?? null),
			trailDistance: text(risk?.trailDistance() ?? null),
			minEdge: text(risk?.minEdge() ?? null),
			maxLoss: text(risk?.maxLoss() ?? null),
		},
		stopFloor: text(stop?.floor() ?? null),
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
			return "How much of the requested quantity the visible order book could actually supply.";
		case "execution:spread":
			return "The fraction of entry price lost to the visible bid/ask gap.";
		case "execution:impact":
			return "The fraction of entry price lost by consuming multiple ask levels.";
		case "execution:friction":
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
		default:
			return "A named fact recorded on the frozen entry decision.";
	}
};

export const evidenceValue = (evidence: DecisionEvidence): string => {
	if (
		evidence.key.startsWith("probability:") ||
		evidence.key.startsWith("execution:")
	) {
		return `${(evidence.value * 100).toFixed(2)}%`;
	}

	if (
		evidence.key.startsWith("features:") ||
		evidence.key === "horizon:ticker_steps"
	) {
		return evidence.value.toFixed(0);
	}

	if (evidence.key.startsWith("return:") && evidence.key.endsWith("_log")) {
		return `${evidence.value.toFixed(6)} log · ${(Math.expm1(evidence.value) * 100).toFixed(2)}% equivalent`;
	}

	return evidence.value.toFixed(6);
};
