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
	symbols: [
		{
			symbol: "DENT/USD",
			upboundPct: 0.3,
			realizedPct: 0.1,
			missedPct: 0.2,
			legs: 2,
			missedLegs: 1,
			opportunities: [
				{
					leg: {
						symbol: "DENT/USD",
						buyAt: "2026-01-01T00:00:00.000Z",
						buyPrice: 100,
						sellAt: "2026-01-01T00:01:00.000Z",
						sellPrice: 120,
						profitPct: 0.2,
					},
					signal: {
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
		},
	],
};

describe("HindsightPanel", () => {
	it("renders ranked causes, blockers, and a concrete next experiment", () => {
		const markup = renderToStaticMarkup(<HindsightPanel report={report} />);

		expect(markup).toContain("Priority experiments");
		expect(markup).toContain("Admission Policy");
		expect(markup).toContain("current 0.5000");
		expect(markup).toContain("counterfactual candidate 0.4800");
		expect(markup).toContain("entry confidence 0.4800 was 0.0200 below");
		expect(markup).toContain("evidence complete");
		expect(markup).toContain("DENT/USD");
	});
});
