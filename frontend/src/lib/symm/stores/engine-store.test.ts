import { describe, expect, it } from "vitest";

import {
	applyDecisionTrace,
	applyEnginePulse,
	engineStore,
} from "#/lib/symm/stores/engine-store";
import type { DecisionTraceEvent, EnginePulseEvent } from "#/lib/symm/events";

describe("engine-store", () => {
	it("stores backend engine pulse and decision trace payloads verbatim", () => {
		const pulse = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:00Z",
			seq: 1,
			phase: "scan",
			measurements: 2,
			candidates: 1,
			open: 0,
			avg_prediction: 0.002,
			avg_error: 0.0005,
			forecast_symbols: 1,
			forecast_errors: 1,
			signals: [
				{
					symbol: "PUMP/EUR",
					source: "hawkes",
					regime: "momentum",
					reason: "cluster_buy",
					score: 0.6,
					type: "momentum",
				},
			],
		} satisfies EnginePulseEvent;

		const trace = {
			event: "decision_trace",
			ts: "2026-05-23T12:00:01Z",
			line: 0.9,
			median: 0.75,
			mad: 0.15,
			scored: 1,
			in_play: 1,
			allowed: 1,
			decisions: [],
			evaluations: [
				{
					symbol: "PUMP/EUR",
					combined: 0.6,
					support: 1,
					regime: "momentum",
					reason: "cluster_buy",
					allow: true,
					why: "ok",
					signals: [
						{
							source: "hawkes",
							regime: "momentum",
							reason: "cluster_buy",
							confidence: 0.6,
						},
					],
				},
			],
		} satisfies DecisionTraceEvent;

		engineStore.setState(() => ({
			pulseLog: [],
			signalConfidences: {
				hawkes: 0,
				fluid: 0,
				pumpdump: 0,
				causal: 0,
			},
		}));
		applyEnginePulse(pulse);
		applyDecisionTrace(trace);

		expect(engineStore.state.enginePulse).toEqual(pulse);
		expect(engineStore.state.decisionTrace).toEqual(trace);
		expect(engineStore.state.pulseLog).toHaveLength(1);
		expect(engineStore.state.signalConfidences).toEqual({
			hawkes: 0.6,
			fluid: 0,
			pumpdump: 0,
			causal: 0,
		});
	});

	it("should keep prior signal confidences when a pulse has no measurements", () => {
		engineStore.setState(() => ({
			pulseLog: [],
			signalConfidences: {
				hawkes: 0,
				fluid: 0,
				pumpdump: 0.726,
				causal: 0,
			},
		}));

		applyEnginePulse({
			event: "engine_pulse",
			ts: "2026-05-23T12:00:02Z",
			seq: 2,
			phase: "scan",
			measurements: 0,
			candidates: 0,
			open: 0,
			signals: [],
		});

		expect(engineStore.state.signalConfidences.pumpdump).toBe(0.726);
	});
});
