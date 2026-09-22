import { describe, expect, it } from "vitest";
import systemGraph from "../../../../manifest/system.json";
import { computeNodeRanks } from "./graphLayout";
import type { NodeMap } from "./types";

describe("computeNodeRanks", () => {
	it("ranks each hop one step further from the source", () => {
		const nodes = {
			a: { connections: { inputs: {}, outputs: {} } },
			b: { connections: { inputs: { in: [{ nodeId: "a", portName: "out" }] } } },
			c: { connections: { inputs: { in: [{ nodeId: "b", portName: "out" }] } } },
		} as unknown as NodeMap;

		const ranks = computeNodeRanks(nodes);

		expect(ranks.get("a")).toBe(0);
		expect(ranks.get("b")).toBe(1);
		expect(ranks.get("c")).toBe(2);
	});

	it("takes the longest path when a node is reachable by two routes", () => {
		const nodes = {
			a: { connections: { inputs: {} } },
			b: { connections: { inputs: { in: [{ nodeId: "a", portName: "out" }] } } },
			c: {
				connections: {
					inputs: {
						short: [{ nodeId: "a", portName: "out" }],
						long: [{ nodeId: "b", portName: "out" }],
					},
				},
			},
		} as unknown as NodeMap;

		expect(computeNodeRanks(nodes).get("c")).toBe(2);
	});

	it("lets a feedback edge close a loop without pushing ranks away", () => {
		const nodes = {
			grid: {
				connections: { inputs: { metrics: [{ nodeId: "signal", portName: "out" }] } },
			},
			signal: {
				connections: { inputs: { data: [{ nodeId: "grid", portName: "out" }] } },
			},
		} as unknown as NodeMap;

		const ranks = computeNodeRanks(nodes);

		expect(Math.max(...ranks.values())).toBeLessThanOrEqual(1);
	});

	it("keeps the real system graph inside a viewable span", () => {
		const nodes = systemGraph.nodes as unknown as NodeMap;
		const ranks = computeNodeRanks(nodes);

		// The grid and the signals feed each other, so an unguarded relaxation
		// runs to its iteration cap and parks every node thousands of pixels
		// off the right of the canvas.
		expect(Math.max(...ranks.values())).toBeLessThan(
			Object.keys(nodes).length,
		);
		expect(Math.max(...ranks.values())).toBeLessThanOrEqual(8);
	});
});
