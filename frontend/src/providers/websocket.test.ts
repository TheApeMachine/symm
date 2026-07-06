import { describe, expect, it } from "vitest";

import { balancesStore } from "#/collections/balances";
import { decisionStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import { measurementsStore } from "#/collections/measurements";
import { routeMessage } from "./websocket";

describe("WsFeed message routing", () => {
	it("routes []Measurement arrays into the measurements store", () => {
		measurementsStore.setState(() => ({
			measurements: {},
			symbols: {},
			sources: new Set<string>(),
			tick: 0,
		}));

		routeMessage([
			{
				source: "fluid",
				symbol: "BTC/USD",
				at: "2026-01-01T00:00:00Z",
				status: "measured",
				elapsed: 1.0,
				entryBaseline: 0.5,
				exitBaseline: 0.3,
				categories: [{ type: "laminar", confidence: 0.8, surprisal: 0.2, strength: 0.6 }],
				metrics: { divergence: 0.1 },
			},
			{
				source: "hawkes",
				symbol: "ETH/USD",
				at: "2026-01-01T00:00:00Z",
				status: "measured",
				elapsed: 0.5,
				entryBaseline: 0.4,
				exitBaseline: 0.2,
				categories: [{ type: "frenzy", confidence: 0.7, surprisal: 0.3, strength: 0.5 }],
				metrics: { branchingRatio: 0.68 },
			},
		]);

		expect(measurementsStore.state.measurements.fluid.values()).toHaveLength(1);
		expect(measurementsStore.state.measurements.hawkes.values()).toHaveLength(1);
		expect(measurementsStore.state.symbols["BTC/USD"]).toHaveLength(1);
		expect(measurementsStore.state.symbols["ETH/USD"]).toHaveLength(1);
		expect(measurementsStore.state.tick).toBe(1);
	});

	it("routes []Action arrays into the decision store", () => {
		decisionStore.actions.reset();

		routeMessage([
			{
				id: "1:BTC/USD:laminar:flow:causal",
				tick: 1,
				symbol: "BTC/USD",
				type: "entry",
				side: "buy",
				verdict: "allow",
				reason: "physical_predictive_causal_match",
				score: 0.72,
				entryLine: 0.5,
				entryScore: 0.8,
				entryConfidence: 0.72,
				fraction: 0.036,
				price: 61420,
				branchKey: "laminar/flow/causal",
				reasonSource: "causal",
				reasonCategory: "trend",
				decisionAt: "2026-01-01T00:00:00Z",
			},
		]);

		const decisions = decisionStore.state.decisions.values();

		expect(decisions).toHaveLength(1);
		expect(decisions[0]?.verdict).toBe("allow");
	});

	it("routes []BalanceData arrays into the balances store", () => {
		balancesStore.actions.reset();

		routeMessage([
			{
				ledger_id: "L1",
				ref_id: "R1",
				asset: "EUR",
				asset_class: "currency",
				amount: 0,
				fee: 0,
				balance: 12840.22,
				available: 9120.55,
				reserved: 3719.67,
			},
		]);

		expect(balancesStore.state.frame).not.toBeNull();
	});

	it("routes []ExecutionData arrays into the executions store", () => {
		executionsStore.actions.reset();

		routeMessage([
			{
				exec_id: "E1",
				exec_type: "trade",
				order_id: "O1",
				symbol: "BTC/EUR",
				side: "buy",
				last_qty: 0.01,
				last_price: 61420,
			},
		]);

		expect(executionsStore.state.frames).toHaveLength(1);
	});

	it("ignores empty arrays", () => {
		const tickBefore = measurementsStore.state.tick;

		routeMessage([]);

		expect(measurementsStore.state.tick).toBe(tickBefore);
	});

	it("ignores non-array data", () => {
		const tickBefore = measurementsStore.state.tick;

		routeMessage({ unknown: "shape" });

		expect(measurementsStore.state.tick).toBe(tickBefore);
	});
});
