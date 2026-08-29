import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { HindsightReport } from "#/collections/app";

import { HindsightPanel } from "./hindsight";

const report: HindsightReport = {
	captureId: 42,
	status: "ready",
	missedPct: 0.2,
	priceTheoreticalCeiling: 0.3,
	executableCeiling: 0.28,
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
			priceTheoreticalCeiling: 0.3,
			executableCeiling: 0.28,
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
						opportunity: true,
						opportunityType: "pump",
						valuationAttempted: true,
						valuationAvailable: false,
						valuationStatus: "incomplete",
						alternatives: {
							"admission:confidence_margin": -0.02,
						},
					},
					regret: {
						detection: false,
						valuation: true,
						selection: false,
						execution: false,
						management: false,
					},
					diagnosis: {
						category: "valuation",
						summary: "missed 100→120: valuation was attempted but no economic consequence was available",
						evidenceQuality: 0.86,
						evidenceStatus: "partial",
						blockers: [
							{
								key: "valuation:not_available",
								category: "valuation",
								label: "valuation unavailable",
								observed: 0,
								target: 0,
								hasTarget: false,
								gap: 0.2,
								severity: 1,
								explanation:
									"valuation was attempted but no economic consequence was available",
							},
						],
						recommendation: {
							key: "valuation:not_available",
							kind: "collect_outcomes",
							target: "valuation evidence",
							title: "Resolve valuation evidence before selection",
							action:
								"Retain whether valuation was attempted and available.",
							rationale: "Economic consequence was not estimable.",
							current: 0,
							suggested: 0,
							hasCurrent: false,
							hasSuggested: false,
							adjustment: "",
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
		expect(markup).toContain("Valuation");
		expect(markup).toContain("Whipsaw Stopout");
		expect(markup).toContain("Where capital was lost");
		expect(markup).toContain("valuation was attempted but no economic consequence was available");
		expect(markup).toContain("price-theoretical ceiling");
		expect(markup).toContain("executable ceiling");
		expect(markup).toContain("valuation unavailable");
		expect(markup).toContain("DENT/USD");
		expect(markup).toContain("gross 20.00%");
		expect(markup).toContain("friction -2.00%");
		expect(markup).toContain("Losing positions post-mortem");
		expect(markup).toContain("stoploss: floor breached at 114.00");
	});
});