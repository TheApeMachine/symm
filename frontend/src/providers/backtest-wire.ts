import type {
	BacktestState,
	HindsightBlocker,
	HindsightDiagnosis,
	HindsightLoss,
	HindsightOpportunity,
	HindsightRecommendation,
	HindsightReport,
	HindsightRootCause,
	HindsightSignal,
	HindsightSymbol,
} from "#/collections/app";
import type { BacktestFrame } from "#/providers/telemetry/telemetry/backtest-frame";
import type { HindsightBlockerT } from "#/providers/telemetry/telemetry/hindsight-blocker";
import type { HindsightDiagnosisT } from "#/providers/telemetry/telemetry/hindsight-diagnosis";
import type { HindsightFrame } from "#/providers/telemetry/telemetry/hindsight-frame";
import type { HindsightLegT } from "#/providers/telemetry/telemetry/hindsight-leg";
import type { HindsightLossT } from "#/providers/telemetry/telemetry/hindsight-loss";
import type { HindsightOpportunityT } from "#/providers/telemetry/telemetry/hindsight-opportunity";
import type { HindsightRecommendationT } from "#/providers/telemetry/telemetry/hindsight-recommendation";
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
	graphScore: signal.graphScore,
	thesisScore: signal.thesisScore,
	thesisConfidence: signal.thesisConfidence,
	thesisSupport: signal.thesisSupport,
	thesisContradiction: signal.thesisContradiction,
	thesisConditions: signal.thesisConditions,
	direction: signal.direction,
	confidence: signal.confidence,
	admissionThreshold: signal.admissionThreshold,
	opportunity: signal.opportunity,
	opportunityType: text(signal.opportunityType),
	predictiveReady: signal.predictiveReady,
	predictiveStatus: text(signal.predictiveStatus),
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

const opportunityToReport = (
	opportunity: HindsightOpportunityT,
): HindsightOpportunity => ({
	leg: legToReport(opportunity.leg ?? ({} as HindsightLegT)),
	signal: signalToReport(opportunity.signal ?? ({} as HindsightSignalT)),
	journal: opportunity.journal.map(signalToReport),
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
	upboundPct: symbol.upboundPct,
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
		missedPct: unpacked.missedPct,
		upboundPct: unpacked.upboundPct,
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
