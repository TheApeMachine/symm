import type {
	BacktestState,
	HindsightBlocker,
	HindsightDiagnosis,
	HindsightExecutable,
	HindsightLoss,
	HindsightOpportunity,
	HindsightRecommendation,
	HindsightRegret,
	HindsightReport,
	HindsightRootCause,
	HindsightSignal,
	HindsightSymbol,
} from "#/collections/app";
import type { BacktestFrame } from "#/providers/telemetry/telemetry/backtest-frame";
import type { HindsightBlockerT } from "#/providers/telemetry/telemetry/hindsight-blocker";
import type { HindsightDiagnosisT } from "#/providers/telemetry/telemetry/hindsight-diagnosis";
import type { HindsightExecutableT } from "#/providers/telemetry/telemetry/hindsight-executable";
import type { HindsightFrame } from "#/providers/telemetry/telemetry/hindsight-frame";
import type { HindsightLegT } from "#/providers/telemetry/telemetry/hindsight-leg";
import type { HindsightLossT } from "#/providers/telemetry/telemetry/hindsight-loss";
import type { HindsightOpportunityT } from "#/providers/telemetry/telemetry/hindsight-opportunity";
import type { HindsightRecommendationT } from "#/providers/telemetry/telemetry/hindsight-recommendation";
import type { HindsightRegretT } from "#/providers/telemetry/telemetry/hindsight-regret";
import type { HindsightRootCauseT } from "#/providers/telemetry/telemetry/hindsight-root-cause";
import type { HindsightSignalT } from "#/providers/telemetry/telemetry/hindsight-signal";
import type { HindsightSymbolT } from "#/providers/telemetry/telemetry/hindsight-symbol";
import type { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";

/*
The backend ships int64 timestamps as UnixNano and int64 counts as bigint over
FlatBuffers. The dashboard's plain state stores ISO-8601 strings and JS numbers,
so every conversion funnels through these two helpers.
*/
const nanosToIso = (nanos: bigint): string | null =>
	nanos === 0n ? null : new Date(Number(nanos / 1_000_000n)).toISOString();

const count = (value: bigint): number => Number(value);

const text = (value: string | Uint8Array | null): string =>
	typeof value === "string" ? value : "";

const alternativesToRecord = (
	alternatives: NamedNumberT[],
): Record<string, number> | null => {
	if (alternatives.length === 0) {
		return null;
	}

	const result: Record<string, number> = {};

	for (const entry of alternatives) {
		result[text(entry.name)] = entry.value;
	}

	return result;
};

const signalToReport = (signal: HindsightSignalT): HindsightSignal => ({
	id: text(signal.id),
	at: nanosToIso(signal.at),
	action: text(signal.action),
	reason: text(signal.reason),
	cause: text(signal.cause),
	opportunity: signal.opportunity,
	opportunityType: text(signal.opportunityType),
	opportunityPhase: text(signal.opportunityPhase),
	valuationAttempted: signal.valuationAttempted,
	valuationAvailable: signal.valuationAvailable,
	valuationStatus: text(signal.valuationStatus),
	causalIdentification: text(signal.causalIdentification),
	causalBlockingCoordinate: text(signal.causalBlockingCoordinate),
	utility: signal.utility,
	utilityAvailable: signal.utilityAvailable,
	proposedQuantity: signal.proposedQuantity,
	proposedNotional: signal.proposedNotional,
	availableCapital: signal.availableCapital,
	allocationClass: text(signal.allocationClass),
	allocationHaircut: signal.allocationHaircut,
	expectedReturn: signal.expectedReturn,
	expectedFees: signal.expectedFees,
	expectedSpread: signal.expectedSpread,
	expectedImpact: signal.expectedImpact,
	adverseSelection: signal.adverseSelection,
	uncertainty: signal.uncertainty,
	openPositions: count(signal.openPositions),
	slotCapacity: count(signal.slotCapacity),
	entryCost: signal.entryCost
		? {
				entryPrice: signal.entryCost.entryPrice,
				bestAsk: signal.entryCost.bestAsk,
				bestBid: signal.entryCost.bestBid,
				grossNotional: signal.entryCost.grossNotional,
				entryFee: signal.entryCost.entryFee,
				spread: signal.entryCost.spread,
				impact: signal.entryCost.impact,
				breakEven: signal.entryCost.breakEven,
			}
		: null,
	risk: signal.risk
		? {
				present: signal.risk.present,
				riskDistance: signal.risk.riskDistance,
				maxLoss: signal.risk.maxLoss,
				entryFeeRate: signal.risk.entryFeeRate,
				exitFeeRate: signal.risk.exitFeeRate,
			}
		: null,
	mcts: signal.mcts
		? {
				recommendedAction: text(signal.mcts.recommendedAction),
				iterations: count(signal.mcts.iterations),
				branches: (signal.mcts.branches ?? []).map((branch) => ({
					action: text(branch.action),
					visits: count(branch.visits),
					meanReward: branch.meanReward,
				})),
			}
		: null,
	alternatives: alternativesToRecord(signal.alternatives),
});

const blockerToReport = (blocker: HindsightBlockerT): HindsightBlocker => ({
	key: text(blocker.key),
	category: text(blocker.category),
	label: text(blocker.label),
	source: text(blocker.source),
	observed: blocker.observed,
	target: blocker.target,
	hasTarget: blocker.hasTarget,
	gap: blocker.gap,
	severity: blocker.severity,
	explanation: text(blocker.explanation),
});

const recommendationToReport = (
	recommendation: HindsightRecommendationT,
): HindsightRecommendation => ({
	key: text(recommendation.key),
	kind: text(recommendation.kind),
	target: text(recommendation.target),
	title: text(recommendation.title),
	action: text(recommendation.action),
	rationale: text(recommendation.rationale),
	current: recommendation.current,
	suggested: recommendation.suggested,
	hasCurrent: recommendation.hasCurrent,
	hasSuggested: recommendation.hasSuggested,
	adjustment: text(recommendation.adjustment),
	confidence: recommendation.confidence,
	impactPct: recommendation.impactPct,
	occurrences: count(recommendation.occurrences),
	symbols: recommendation.symbols ?? [],
});

const rootCauseToReport = (
	cause: HindsightRootCauseT,
): HindsightRootCause => ({
	category: text(cause.category),
	impactPct: cause.impactPct,
	occurrences: count(cause.occurrences),
	symbols: cause.symbols ?? [],
});

const diagnosisToReport = (
	diagnosis: HindsightDiagnosisT | null,
): HindsightDiagnosis | null => {
	if (diagnosis === null) {
		return null;
	}

	return {
		category: text(diagnosis.category),
		summary: text(diagnosis.summary),
		evidenceQuality: diagnosis.evidenceQuality,
		evidenceStatus: text(diagnosis.evidenceStatus),
		blockers: diagnosis.blockers.map(blockerToReport),
		recommendation:
			diagnosis.recommendation === null
				? null
				: recommendationToReport(diagnosis.recommendation),
	};
};

const legToReport = (leg: HindsightLegT) => ({
	symbol: text(leg.symbol),
	buyAt: nanosToIso(leg.buyAt) ?? "",
	sellAt: nanosToIso(leg.sellAt) ?? "",
	buyPrice: leg.buyPrice,
	sellPrice: leg.sellPrice,
	profitPct: leg.profitPct,
	grossProfitPct: leg.grossProfitPct,
	frictionPct: leg.frictionPct,
});

const executableToReport = (
	executable: HindsightExecutableT | null,
): HindsightExecutable | null => {
	if (executable === null) {
		return null;
	}

	return {
		symbol: text(executable.symbol),
		buyAt: nanosToIso(executable.buyAt) ?? "",
		sellAt: nanosToIso(executable.sellAt) ?? "",
		theoreticalBuyPrice: executable.theoreticalBuyPrice,
		theoreticalSellPrice: executable.theoreticalSellPrice,
		theoreticalReturn: executable.theoreticalReturn,
		requestedQty: executable.requestedQty,
		requestedNotional: executable.requestedNotional,
		executableEntryQty: executable.executableEntryQty,
		executableEntryVWAP: executable.executableEntryVwap,
		executableEntryValue: executable.executableEntryValue,
		executableEntryFees: executable.executableEntryFees,
		entryImpact: executable.entryImpact,
		executableExitQty: executable.executableExitQty,
		executableExitVWAP: executable.executableExitVwap,
		executableExitValue: executable.executableExitValue,
		executableExitFees: executable.executableExitFees,
		exitImpact: executable.exitImpact,
		fullyExecutable: executable.fullyExecutable,
		executableReturn: executable.executableReturn,
		executablePnL: executable.executablePnL,
	};
};

const regretToReport = (
	regret: HindsightRegretT | null,
): HindsightRegret | null => {
	if (regret === null) {
		return null;
	}

	return {
		detection: regret.detection,
		valuation: regret.valuation,
		selection: regret.selection,
		execution: regret.execution,
		management: regret.management,
	};
};

const opportunityToReport = (
	opportunity: HindsightOpportunityT,
): HindsightOpportunity => ({
	leg: legToReport(opportunity.leg ?? ({} as HindsightLegT)),
	signal: signalToReport(opportunity.signal ?? ({} as HindsightSignalT)),
	journal: opportunity.journal.map(signalToReport),
	executable: executableToReport(opportunity.executable),
	regret: regretToReport(opportunity.regret),
	why: text(opportunity.why),
	captured: opportunity.captured,
	missed: opportunity.missed,
	diagnosis: diagnosisToReport(opportunity.diagnosis),
});

const lossToReport = (loss: HindsightLossT): HindsightLoss => ({
	symbol: text(loss.symbol),
	decisionId: text(loss.decisionId),
	entryAt: nanosToIso(loss.entryAt),
	exitAt: nanosToIso(loss.exitAt),
	entryPrice: loss.entryPrice,
	exitPrice: loss.exitPrice,
	lossPerUnit: loss.pnl,
	returnPct: loss.returnPct,
	grossPct: loss.grossPct,
	frictionPct: loss.frictionPct,
	triggerReason: text(loss.triggerReason),
	diagnosis: diagnosisToReport(loss.diagnosis),
	signal: signalToReport(loss.signal ?? ({} as HindsightSignalT)),
	journal: loss.journal.map(signalToReport),
});

const symbolToReport = (symbol: HindsightSymbolT): HindsightSymbol => ({
	symbol: text(symbol.symbol),
	priceTheoreticalCeiling: symbol.priceTheoreticalCeiling,
	executableCeiling: symbol.executableCeiling,
	executableLegsDefined: count(symbol.executableLegsDefined),
	executableLegsTotal: count(symbol.executableLegsTotal),
	realizedPct: symbol.realizedPct,
	missedPct: symbol.missedPct,
	lossPct: symbol.lossPct,
	legs: count(symbol.legs),
	missedLegs: count(symbol.missedLegs),
	lossPositions: count(symbol.lossPositions),
	opportunities: symbol.opportunities.map(opportunityToReport),
	losses: symbol.losses.map(lossToReport),
});

/*
backtestFrameToState projects one BacktestFrame wire value onto the plain
BacktestState the dashboard renders. Zero timestamps mean "no bound yet" and
become null, matching the store's initial shape.
*/
export const backtestFrameToState = (
	frame: BacktestFrame,
): Partial<BacktestState> => {
	const captureId = frame.captureId();

	return {
		captureId: captureId === 0n ? null : count(captureId),
		playing: frame.playing(),
		position: nanosToIso(frame.position()),
		startedAt: nanosToIso(frame.startedAt()),
		endedAt: nanosToIso(frame.endedAt()),
		rebooting: frame.rebooting(),
	};
};

/*
hindsightFrameToReport converts the full nested HindsightFrame wire value into
the plain HindsightReport the panel renders.
*/
export const hindsightFrameToReport = (
	frame: HindsightFrame,
): HindsightReport => {
	const unpacked = frame.unpack();

	return {
		captureId: count(unpacked.captureId),
		status: text(unpacked.status),
		symbols: unpacked.symbols.map(symbolToReport),
		priceTheoreticalCeiling: unpacked.priceTheoreticalCeiling,
		executableCeiling: unpacked.executableCeiling,
		missedPct: unpacked.missedPct,
		missedLegs: count(unpacked.missedLegs),
		totalLegs: count(unpacked.totalLegs),
		realizedPct: unpacked.realizedPct,
		lossPct: unpacked.lossPct,
		lossPositions: count(unpacked.lossPositions),
		valueCaptureRate: unpacked.valueCaptureRate,
		legCaptureRate: unpacked.legCaptureRate,
		diagnosticCoverage: unpacked.diagnosticCoverage,
		rootCauses: unpacked.rootCauses.map(rootCauseToReport),
		recommendations: unpacked.recommendations.map(recommendationToReport),
		lossRootCauses: unpacked.lossRootCauses.map(rootCauseToReport),
		lossRecommendations: unpacked.lossRecommendations.map(
			recommendationToReport,
		),
	};
};
