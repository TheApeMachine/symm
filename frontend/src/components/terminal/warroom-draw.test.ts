import { describe, expect, it } from "vitest";
import type { TraceNode } from "#/components/terminal/decision-trace-model";
import {
	layoutWarRoomTree,
	warRoomTreeFrom,
} from "#/components/terminal/warroom-draw";

const node = (overrides: Partial<TraceNode> = {}): TraceNode => ({
	action: "",
	depth: 0,
	visits: 0,
	effectiveVisits: 0,
	meanReward: 0,
	rewardStd: 0,
	blendedValue: 0,
	counterfactualReward: 0,
	counterfactualMass: 0,
	causalExpectation: 0,
	causalExpectationDefined: false,
	pruned: false,
	selected: false,
	children: [],
	...overrides,
});

describe("warRoomTreeFrom", () => {
	it("has nothing to draw when no search ran", () => {
		const tree = warRoomTreeFrom(null);

		expect(tree.root).toBeNull();
		expect(tree.nodes).toHaveLength(0);
	});

	it("follows the path the search itself selected, not the busiest one", () => {
		// The busiest branch and the selected branch differ here: the search's
		// causal bias picked the lighter one. Re-deriving the path from visits
		// would draw a route the search never took.
		const tree = warRoomTreeFrom(
			node({
				action: "root",
				visits: 30,
				children: [
					node({ action: "hold", depth: 1, visits: 25 }),
					node({ action: "enter", depth: 1, visits: 5, selected: true }),
				],
			}),
		);

		const onPath = tree.nodes
			.filter((entry) => entry.onPath)
			.map((entry) => entry.node.action);

		expect(onPath).toEqual(["root", "enter"]);
	});

	it("reports the peaks the drawing scales against", () => {
		const tree = warRoomTreeFrom(
			node({
				action: "root",
				visits: 8,
				children: [
					node({ action: "enter", depth: 1, visits: 3, counterfactualMass: 7 }),
				],
			}),
		);

		expect(tree.peakVisits).toBe(8);
		expect(tree.peakMass).toBe(7);
		expect(tree.maxDepth).toBe(1);
	});

	it("keeps a pruned branch in the tree so rejection stays visible", () => {
		// A causally rejected branch must still be drawn — struck through —
		// because "the model condemned this" is evidence, not absence.
		const tree = warRoomTreeFrom(
			node({
				action: "root",
				children: [node({ action: "enter", depth: 1, pruned: true })],
			}),
		);

		expect(tree.nodes).toHaveLength(2);
		expect(tree.nodes[1]?.node.pruned).toBe(true);
	});
});

describe("layoutWarRoomTree", () => {
	it("places depth on x and keeps every node inside the box", () => {
		const tree = warRoomTreeFrom(
			node({
				action: "root",
				children: [
					node({
						action: "enter",
						depth: 1,
						children: [node({ action: "hold", depth: 2 })],
					}),
				],
			}),
		);

		const layout = layoutWarRoomTree(tree, 600, 300);

		const rootX = layout.x.get(0) ?? 0;
		const midX = layout.x.get(1) ?? 0;
		const leafX = layout.x.get(2) ?? 0;

		expect(rootX).toBeLessThan(midX);
		expect(midX).toBeLessThan(leafX);

		for (const entry of tree.nodes) {
			expect(layout.x.get(entry.id)).toBeLessThanOrEqual(600);
			expect(layout.y.get(entry.id)).toBeLessThanOrEqual(300);
			expect(layout.y.get(entry.id)).toBeGreaterThanOrEqual(0);
		}
	});

	it("centres a parent on the children it leads to", () => {
		const tree = warRoomTreeFrom(
			node({
				action: "root",
				children: [
					node({ action: "a", depth: 1 }),
					node({ action: "b", depth: 1 }),
				],
			}),
		);

		const layout = layoutWarRoomTree(tree, 400, 200);
		const parent = layout.y.get(0) ?? 0;
		const first = layout.y.get(1) ?? 0;
		const second = layout.y.get(2) ?? 0;

		expect(parent).toBeCloseTo((first + second) / 2, 5);
	});

	it("survives an empty tree without producing coordinates", () => {
		const layout = layoutWarRoomTree(warRoomTreeFrom(null), 400, 200);

		expect(layout.x.size).toBe(0);
	});
});
