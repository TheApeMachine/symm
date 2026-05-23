import { describe, expect, it } from "vitest";

import {
	emptyEvaluationState,
	mergeDecisionTrace,
	mergeEnginePulse,
	rankedEvaluations,
} from "#/lib/symm/evaluation-store";
import type { DecisionTraceEvent, EnginePulseEvent } from "#/lib/symm/events";

describe("evaluation-store", () => {
	it("merges engine pulse readings per symbol and source", () => {
		const pulse = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:00Z",
			seq: 1,
			phase: "scan",
			measurements: 2,
			candidates: 1,
			open: 0,
			signals: [
				{
					symbol: "PUMP/EUR",
					source: "hawkes",
					regime: "momentum",
					reason: "cluster_buy",
					score: 0.6,
					type: "momentum",
				},
				{
					symbol: "PUMP/EUR",
					source: "fluid",
					regime: "flow",
					reason: "accumulation",
					score: 0.4,
					type: "flow",
				},
			],
		} satisfies EnginePulseEvent;

		const state = mergeEnginePulse(emptyEvaluationState(), pulse);
		const row = state.bySymbol.get("PUMP/EUR");

		expect(row?.signals.hawkes.confidence).toBe(0.6);
		expect(row?.signals.fluid.confidence).toBe(0.4);
		expect(state.seq).toBe(1);
	});

	it("merges decision trace evaluations and ranks by combined score", () => {
		const trace = {
			event: "decision_trace",
			ts: "2026-05-23T12:00:01Z",
			line: 0.9,
			median: 0.75,
			mad: 0.15,
			scored: 2,
			in_play: 2,
			allowed: 1,
			decisions: [],
			evaluations: [
				{
					symbol: "A/EUR",
					combined: 0.5,
					support: 1,
					regime: "momentum",
					reason: "cluster_buy",
					allow: false,
					why: "below_line",
					signals: [
						{
							source: "hawkes",
							regime: "momentum",
							reason: "cluster_buy",
							confidence: 0.5,
						},
					],
				},
				{
					symbol: "B/EUR",
					combined: 1.1,
					support: 2,
					regime: "pump",
					reason: "actual_pump",
					allow: true,
					why: "ok",
					signals: [
						{
							source: "pumpdump",
							regime: "pump",
							reason: "actual_pump",
							confidence: 0.7,
						},
						{
							source: "hawkes",
							regime: "momentum",
							reason: "cluster_buy",
							confidence: 0.4,
						},
					],
				},
			],
		} satisfies DecisionTraceEvent;

		const state = mergeDecisionTrace(emptyEvaluationState(), trace);
		const ranked = rankedEvaluations(state);

		expect(state.line).toBe(0.9);
		expect(ranked[0]?.symbol).toBe("B/EUR");
		expect(ranked[1]?.allow).toBe(false);
	});
});
