import { describe, expect, it } from "vitest";

import type { DecisionTraceEvent, EnginePulseEvent } from "#/lib/symm/events";
import {
	applyDecisionTrace,
	applyEnginePulse,
	applyFieldAggregate,
	applyFieldGrid,
	applyFieldRow,
	applySignalScore,
	buildFieldSnapshot,
	dashboardStore,
	setFeedConnected,
} from "#/lib/symm/dashboard-store";
import { emptySignalConfidences } from "#/lib/symm/signal-confidence";

describe("dashboard-store", () => {
	it("tracks websocket connection state", () => {
		dashboardStore.setState((state) => ({ ...state, connected: false }));

		setFeedConnected(true);
		expect(dashboardStore.state.connected).toBe(true);

		setFeedConnected(true);
		expect(dashboardStore.state.connected).toBe(true);

		setFeedConnected(false);
		expect(dashboardStore.state.connected).toBe(false);
	});

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
			evaluations: [],
		} satisfies DecisionTraceEvent;

		dashboardStore.setState((state) => ({
			...state,
			pulseLog: [],
			signalConfidences: emptySignalConfidences(),
		}));
		applyEnginePulse(pulse);
		applyDecisionTrace(trace);

		expect(dashboardStore.state.enginePulse).toEqual(pulse);
		expect(dashboardStore.state.decisionTrace).toEqual(trace);
		expect(dashboardStore.state.pulseLog).toHaveLength(1);
	});

	it("updates one gauge reading from signal_score events", () => {
		dashboardStore.setState((state) => ({
			...state,
			pulseLog: [],
			signalConfidences: emptySignalConfidences(),
		}));

		applySignalScore({
			event: "signal_score",
			ts: "2026-05-23T12:00:02Z",
			source: "pumpdump",
			confidence: 0.726,
		});

		expect(dashboardStore.state.signalConfidences).toEqual({
			hawkes: 0,
			fluid: 0,
			pumpdump: 0.726,
			causal: 0,
		});
	});

	it("merges field_row and field_aggregate incrementally", () => {
		dashboardStore.setState((state) => ({
			...state,
			rows: {},
			symbolCount: 0,
		}));

		applyFieldRow({
			symbol: "BTC/EUR",
			change_pct: 1.2,
			vol: 0.5,
			div: 0.1,
			vort: 0.2,
			turb: 0.3,
			visc: 0.4,
			re: 42,
		});

		applyFieldAggregate(1, {
			re: 42,
			vort: 0.2,
			div: 0.1,
			turb: 0.3,
			visc: 0.4,
		});

		const snapshot = buildFieldSnapshot(dashboardStore.state);

		expect(snapshot?.symbol_count).toBe(1);
		expect(snapshot?.field.re).toBe(42);
		expect(snapshot?.symbols).toHaveLength(1);
	});

	it("applies field_grid without clearing rows", () => {
		dashboardStore.setState((state) => ({
			...state,
			rows: {
				"ETH/EUR": {
					symbol: "ETH/EUR",
					change_pct: 0,
					vol: 1,
					div: 0,
					vort: 0,
					turb: 0,
					visc: 0,
					re: 10,
				},
			},
			symbolCount: 1,
		}));

		applyFieldGrid({
			size: 2,
			heights: [
				[0, 1],
				[1, 0],
			],
			min: 0,
			max: 1,
			filled_cells: 2,
			outliers: {
				clipped_count: 0,
				clipped_at: 0,
				raw_max: 1,
				display_max: 1,
			},
		});

		const snapshot = buildFieldSnapshot(dashboardStore.state);

		expect(snapshot?.grid?.filled_cells).toBe(2);
		expect(snapshot?.symbols).toHaveLength(1);
	});
});
