import { describe, expect, it } from "vitest";

import {
	buildGraphState,
	formatGraphCaption,
} from "#/components/symm/causal-graph-layout";

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
	confounding_gap: 0.05,
	confidence: 0.4,
	reason: "intervention",
	coef_macro: 0.1,
	coef_liquidity: 0.05,
	coef_flow: 0.35,
	peers: [{ symbol: "ETH/EUR", correlation: 0.82 }],
};

describe("buildGraphState", () => {
	it("adds pearl ladder and peer nodes", () => {
		const { nodes, edges } = buildGraphState(sampleRow);

		expect(nodes).toHaveLength(7 + sampleRow.peers.length);
		expect(edges.some((edge) => edge.kind === "ladder")).toBe(true);
		expect(edges.some((edge) => edge.kind === "peer")).toBe(true);
		expect(nodes[6]?.label).toContain("L3");
		expect(nodes[7]?.label).toContain("ETH");
	});
});

describe("formatGraphCaption", () => {
	it("includes pearl ladder and peers", () => {
		const caption = formatGraphCaption(sampleRow);

		expect(caption).toContain("L1");
		expect(caption).toContain("L2");
		expect(caption).toContain("L3");
		expect(caption).toContain("ETH");
	});
});
