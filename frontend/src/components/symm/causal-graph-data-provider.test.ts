import { describe, expect, it } from "vitest";

import {
	CausalGraphDataProvider,
	isCausalGraphRow,
} from "#/components/symm/causal-graph-data-provider";

const sampleRow = {
	event: "causal_graph" as const,
	symbol: "BTC/EUR",
	ready: true,
	sample_count: 12,
	macro_momentum: 0.01,
	local_flow: 42,
	liquidity: 2.5,
	price_velocity: 0.002,
	association: 0.3,
	intervention: 0.25,
	uplift: 0.05,
	confidence: 0.4,
	reason: "intervention",
	coef_macro: 0.1,
	coef_liquidity: 0.05,
	coef_flow: 0.35,
	confounding_gap: 0.05,
	peers: [{ symbol: "ETH/EUR", correlation: 0.82 }],
};

describe("isCausalGraphRow", () => {
	it("accepts hub causal graph payloads", () => {
		expect(isCausalGraphRow(sampleRow)).toBe(true);
	});

	it("rejects confidence payloads", () => {
		expect(
			isCausalGraphRow({
				source: "causal",
				confidence: 0.5,
				count: 1,
			}),
		).toBe(false);
	});
});

describe("CausalGraphDataProvider", () => {
	it("routes graph rows by symbol", () => {
		const received: string[] = [];
		const unregister = CausalGraphDataProvider.registerSymbol(
			"BTC/EUR",
			(row) => {
				received.push(row.reason);
			},
		);

		CausalGraphDataProvider.ingest(sampleRow);

		expect(received).toEqual(["intervention"]);
		unregister();
	});
});
