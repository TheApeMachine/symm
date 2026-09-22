import { describe, expect, it } from "vitest";
import systemGraph from "../../../../manifest/system.json";
import {
	computeNodeRanks,
	estimateNodeHeight,
	GRID_CELL_SIZE,
	MIN_VERTICAL_GAP,
	optimizeOrthogonalLayout,
} from "./graphLayout";
import type { NodeMap } from "./types";

describe("computeNodeRanks", () => {
	it("ranks each hop one step further from the source", () => {
		const nodes = {
			a: { connections: { inputs: {}, outputs: {} } },
			b: {
				connections: { inputs: { in: [{ nodeId: "a", portName: "out" }] } },
			},
			c: {
				connections: { inputs: { in: [{ nodeId: "b", portName: "out" }] } },
			},
		} as unknown as NodeMap;

		const ranks = computeNodeRanks(nodes);

		expect(ranks.get("a")).toBe(0);
		expect(ranks.get("b")).toBe(1);
		expect(ranks.get("c")).toBe(2);
	});

	it("takes the longest path when a node is reachable by two routes", () => {
		const nodes = {
			a: { connections: { inputs: {} } },
			b: {
				connections: { inputs: { in: [{ nodeId: "a", portName: "out" }] } },
			},
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
				connections: {
					inputs: { metrics: [{ nodeId: "signal", portName: "out" }] },
				},
			},
			signal: {
				connections: {
					inputs: { data: [{ nodeId: "grid", portName: "out" }] },
				},
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
		expect(Math.max(...ranks.values())).toBeLessThan(Object.keys(nodes).length);
		expect(Math.max(...ranks.values())).toBeLessThanOrEqual(8);
	});
});

describe("optimizeOrthogonalLayout", () => {
	it("snaps all coordinates to the 32px grid", () => {
		const nodes = {
			sourceNode: {
				type: "source",
				connections: {
					inputs: {},
					outputs: { out: [{ nodeId: "consumerNode", portName: "in" }] },
				},
			},
			consumerNode: {
				type: "sink",
				connections: {
					inputs: { in: [{ nodeId: "sourceNode", portName: "out" }] },
					outputs: {},
				},
			},
		} as unknown as NodeMap;

		const updates = optimizeOrthogonalLayout(nodes);

		expect(updates.length).toBe(2);
		for (const update of updates) {
			expect(update.x % GRID_CELL_SIZE).toBe(0);
			expect(update.y % GRID_CELL_SIZE).toBe(0);
		}
	});

	it("maintains non-overlapping vertical gaps between nodes in the same rank", () => {
		const nodes = {
			sourceAlpha: {
				type: "source",
				connections: { inputs: {}, outputs: {} },
			},
			sourceBeta: {
				type: "source",
				connections: { inputs: {}, outputs: {} },
			},
		} as unknown as NodeMap;

		const updates = optimizeOrthogonalLayout(nodes);
		const coordAlpha = updates.find((item) => item.nodeId === "sourceAlpha");
		const coordBeta = updates.find((item) => item.nodeId === "sourceBeta");

		expect(coordAlpha).toBeDefined();
		expect(coordBeta).toBeDefined();

		if (!coordAlpha || !coordBeta) {
			throw new Error("Coordinates must be defined");
		}

		const [firstNode, secondNode] =
			coordAlpha.y < coordBeta.y
				? [coordAlpha, coordBeta]
				: [coordBeta, coordAlpha];

		const firstHeight = estimateNodeHeight(nodes[firstNode.nodeId]);
		expect(secondNode.y).toBeGreaterThanOrEqual(
			firstNode.y + firstHeight + MIN_VERTICAL_GAP,
		);
	});

	it("arranges left-to-right columns matching topological dependency flow", () => {
		const nodes = {
			stepOne: {
				type: "producer",
				connections: {
					inputs: {},
					outputs: { out: [{ nodeId: "stepTwo", portName: "in" }] },
				},
			},
			stepTwo: {
				type: "transformer",
				connections: {
					inputs: { in: [{ nodeId: "stepOne", portName: "out" }] },
					outputs: { out: [{ nodeId: "stepThree", portName: "in" }] },
				},
			},
			stepThree: {
				type: "consumer",
				connections: {
					inputs: { in: [{ nodeId: "stepTwo", portName: "out" }] },
					outputs: {},
				},
			},
		} as unknown as NodeMap;

		const updates = optimizeOrthogonalLayout(nodes);
		const map = new Map(updates.map((item) => [item.nodeId, item]));

		const posOne = map.get("stepOne");
		const posTwo = map.get("stepTwo");
		const posThree = map.get("stepThree");

		if (!posOne || !posTwo || !posThree) {
			throw new Error("Positions must be defined");
		}

		expect(posOne.x).toBeLessThan(posTwo.x);
		expect(posTwo.x).toBeLessThan(posThree.x);
	});

	it("successfully optimizes layout for the full system manifest", () => {
		const nodes = systemGraph.nodes as unknown as NodeMap;
		const updates = optimizeOrthogonalLayout(nodes);

		expect(updates.length).toBe(Object.keys(nodes).length);
		for (const update of updates) {
			expect(Number.isFinite(update.x)).toBe(true);
			expect(Number.isFinite(update.y)).toBe(true);
			expect(update.x % GRID_CELL_SIZE).toBe(0);
			expect(update.y % GRID_CELL_SIZE).toBe(0);
		}
	});
});
