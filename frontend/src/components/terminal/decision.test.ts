import { describe, expect, it } from "vitest";
import {
	candidateBarsForRow,
	counterfactualProbes,
	decisionTreeModel,
	kernelsForSymbol,
} from "#/components/terminal/decision";
import type { TerminalDecisionRow } from "#/components/terminal/model";

const row = (symbol = "OP/EUR"): TerminalDecisionRow => ({
	key: `${symbol}:test`,
	symbol,
	source: "decision",
	scoreText: "0.600",
	scoreValue: 0.6,
	verdict: "in-play",
	why: "below line",
	signals: [],
	edgeText: "-0.100 / 0.700",
	edgePositive: false,
});

describe("decision tree page model", () => {
	it("builds the funnel from backend decision frames", () => {
		const model = decisionTreeModel(
			{
				hawkes: {
					"OP/EUR": { output: { confidence: 0.58, surprise: 0.12 } },
					"TON/EUR": { output: { confidence: 0.46, surprise: 0.09 } },
				},
			},
			{
				"OP/EUR": {
					symbol: "OP/EUR",
					active_path: [0, 1],
					steps: [{ path: [0], outcome: "action", reason: "matched_branch" }],
				},
			},
			{
				decisions: [
					{
						symbol: "TON/EUR",
						source: "hawkes",
						verdict: "below",
						why: "below_edge",
						confidence: 0.46,
					},
				],
			},
			{
				candidate_count: 3,
				admitted_count: 1,
				submitted_count: 1,
				open_order_count: 0,
				first_blocker: "edge_unavailable",
			},
			{ quotes_total: 24, quotes_ready: 24 },
		);

		expect(model.funnel.map((card) => card.label)).toEqual([
			"Candidates",
			"Admitted",
			"Submitted",
			"Open",
		]);
		expect(model.funnel.map((card) => card.value)).toEqual([3, 1, 1, 0]);
		expect(model.rows.map((entry) => entry.symbol)).toEqual(["TON/EUR"]);
	});

	it("keeps rows from the rolling backend decision frame window", () => {
		const model = decisionTreeModel(
			{},
			{},
			[
				{
					role: "decisions",
					tick: 10,
					decisions: [
						{
							symbol: "ETH/USD",
							verdict: "allow",
							score: 0.62,
							tick: 10,
						},
					],
				},
				{ role: "decisions", tick: 11, decisions: [] },
			],
			null,
			{ quotes_total: 1, quotes_ready: 1 },
		);

		expect(model.rows.map((entry) => entry.symbol)).toEqual(["ETH/USD"]);
	});

	it("uses exact-symbol measurements for candidate bars", () => {
		const readings = {
			hawkes: {
				"OP/EUR": { output: { confidence: 0.58, surprise: 0.1 } },
			},
			fluid: {
				"TON/EUR": { output: { confidence: 0.99, surprise: 0.99 } },
			},
		};

		expect(
			kernelsForSymbol(readings, "OP/EUR").map((kernel) => kernel.source),
		).toEqual(["hawkes"]);

		const bars = candidateBarsForRow(row(), readings, 0.6);

		expect(bars.map((bar) => bar.source)).toEqual(["hawkes"]);
		expect(bars[0]?.value).toBe("0.58");
	});

	it("projects counterfactual probes from causal, liquidity, and manifold frames", () => {
		const probes = counterfactualProbes(
			{
				causal: {
					"OP/EUR": { output: { uplift: 0.8, beta: 0.4 } },
				},
				liquidity: {
					"OP/EUR": { output: { confidence: 0.3 } },
				},
				manifold: {
					"OP/EUR": {
						carriers: [{ role: "whale" }, { role: "symbol" }],
					},
				},
			},
			row(),
			0.7,
		);

		expect(probes.map((probe) => probe.label)).toEqual([
			"do(vol ↑)",
			"do(regime = chop)",
			"do(liquidity ↓)",
			"do(whale carrier)",
		]);
		expect(probes[0]).toMatchObject({
			deltaText: "+0.200",
			verdict: "ALLOW",
		});
		expect(probes[2]).toMatchObject({
			deltaText: "-0.300",
			verdict: "BELOW",
		});
	});
});
