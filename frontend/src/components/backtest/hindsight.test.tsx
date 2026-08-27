import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { HindsightReport } from "#/collections/app";

import { HindsightPanel } from "./hindsight";

const report: HindsightReport = {
	captureId: 42,
	status: "ready",
	missedPct: 0.2,
	upboundPct: 0.3,
	missedLegs: 1,
	totalLegs: 2,
	realizedPct: 0.1,
	lossPct: 0.05,
	lossPositions: 1,
	valueCaptureRate: 1 / 3,
	legCaptureRate: 0.5,
	diagnosticCoverage: 1,
	rootCauses: [
		{
			category: "admission_policy",
			impactPct: 0.2,
			occurrences: 1,
			symbols: ["DENT/USD"],
		},
	],
	recommendations: [
		{
			key: "admission:confidence",
			kind: "tune_parameter",
			target: "trading.admission.minimum_confidence",
			title: "Backtest an entry confidence boundary sweep",
			action: "Replay retained no-action decisions and compare recovered value with wallet loss.",
			rationale: "This exact gate stopped the decision.",
			current: 0.5,
			suggested: 0.48,
			hasCurrent: true,
			hasSuggested: true,
			adjustment: "lower",
			confidence: 0.86,
			impactPct: 0.2,
			occurrences: 1,
			symbols: ["DENT/USD"],
		},
	],
	lossRootCauses: [
		{
			category: "whipsaw_stopout",
			impactPct: 0.05,
			occurrences: 1,
			symbols: ["DENT/USD"],
		},
	],
	lossRecommendations: [
		{
			key: "tune_stoploss_buffer",
			kind: "tune_risk",
			target: "trading.risk.stoploss_buffer",
			title: "Tune stoploss buffer",
			action: "Widen stoploss threshold to clear market noise.",
			rationale: "Position stopped out on immediate wick.",
			current: 0.02,
			suggested: 0.035,
			hasCurrent: true,
			hasSuggested: true,
			adjustment: "raise",
			confidence: 0.88,
			impactPct: 0.05,
			occurrences: 1,
			symbols: ["DENT/USD"],
		},
	],
	symbols: [
		{
			symbol: "DENT/USD",
			upboundPct: 0.3,
			realizedPct: 0.1,
			missedPct: 0.2,
			lossPct: 0.05,
			legs: 2,
			missedLegs: 1,
			lossPositions: 1,
			opportunities: [
				{
					leg: {
						symbol: "DENT/USD",
						buyAt: "2026-01-01T00:00:00.000Z",
						buyPrice: 100,
						sellAt: "2026-01-01T00:01:00.000Z",
						sellPrice: 120,
						profitPct: 0.18,
						grossProfitPct: 0.20,
						frictionPct: 0.02,
					},
					signal: {
						id: "signal-1",
						at: "2026-01-01T00:00:00.000Z",
						action: "nothing",
						graphScore: 0.7,
						thesisScore: 0.48,
						thesisConfidence: 0.48,
						opportunity: true,
						opportunityType: "pump",
						alternatives: {
							"admission:confidence_margin": -0.02,
						},
					},
					diagnosis: {
						category: "admission_policy",
						summary: "missed 100→120: confidence was below admission",
						evidenceQuality: 0.86,
						evidenceStatus: "complete",
						blockers: [
							{
								key: "admission:confidence",
								category: "admission_policy",
								label: "entry confidence",
								source: "trading.admission.minimum_confidence",
								observed: 0.48,
								target: 0.5,
								hasTarget: true,
								gap: 0.02,
								severity: 0.02,
								explanation:
									"entry confidence 0.4800 was 0.0200 below the required 0.5000",
							},
						],
						recommendation: {
							key: "admission:confidence",
							kind: "tune_parameter",
							target: "trading.admission.minimum_confidence",
							title: "Backtest an entry confidence boundary sweep",
							action:
								"Replay retained no-action decisions and compare recovered value with wallet loss.",
							rationale: "This exact gate stopped the decision.",
							current: 0.5,
							suggested: 0.48,
							hasCurrent: true,
							hasSuggested: true,
							adjustment: "lower",
							confidence: 0.86,
							impactPct: 0.2,
							occurrences: 1,
							symbols: ["DENT/USD"],
						},
					},
					captured: false,
					missed: true,
				},
			],
			losses: [
				{
					symbol: "DENT/USD",
					decisionId: "dec-loss-1",
					entryAt: "2026-01-01T00:02:00.000Z",
					exitAt: "2026-01-01T00:03:00.000Z",
					entryPrice: 120,
					exitPrice: 114,
					lossPerUnit: -6,
					returnPct: -0.052,
					grossPct: -0.05,
					frictionPct: 0.002,
					triggerReason: "stoploss: floor breached at 114.00",
					signal: {
						id: "signal-loss-1",
						at: "2026-01-01T00:02:00.000Z",
						action: "enter",
						graphScore: 0.8,
						thesisScore: 0.7,
						thesisConfidence: 0.65,
						opportunity: true,
						opportunityType: "pump",
						alternatives: null,
					},
					diagnosis: {
						category: "whipsaw_stopout",
						summary: "Position stopped out by stoploss floor breach.",
						evidenceQuality: 0.88,
						evidenceStatus: "complete",
						blockers: [],
						recommendation: {
							key: "tune_stoploss_buffer",
							kind: "tune_risk",
							target: "trading.risk.stoploss_buffer",
							title: "Tune stoploss buffer",
							action: "Widen stoploss threshold to clear market noise.",
							rationale: "Position stopped out on immediate wick.",
							current: 0.02,
							suggested: 0.035,
							hasCurrent: true,
							hasSuggested: true,
							adjustment: "raise",
							confidence: 0.88,
							impactPct: 0.05,
							occurrences: 1,
							symbols: ["DENT/USD"],
						},
					},
				},
			],
		},
	],
};

describe("HindsightPanel", () => {
	it("renders ranked causes, blockers, concrete experiments, and loss post-mortem", () => {
		const markup = renderToStaticMarkup(<HindsightPanel report={report} />);

		expect(markup).toContain("Priority opportunity experiments");
		expect(markup).toContain("Priority risk &amp; execution fixes");
		expect(markup).toContain("Admission Policy");
		expect(markup).toContain("Whipsaw Stopout");
		expect(markup).toContain("Where capital was lost");
		expect(markup).toContain("current 0.5000");
		expect(markup).toContain("counterfactual candidate 0.4800");
		expect(markup).toContain("entry confidence 0.4800 was 0.0200 below");
		expect(markup).toContain("evidence complete");
		expect(markup).toContain("DENT/USD");
		expect(markup).toContain("gross 20.00%");
		expect(markup).toContain("friction -2.00%");
		expect(markup).toContain("Losing positions post-mortem");
		expect(markup).toContain("stoploss: floor breached at 114.00");
	});
});