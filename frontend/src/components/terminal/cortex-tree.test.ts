import { describe, expect, it } from "vitest";
import {
	CortexLeafRoster,
	displayToken,
	layoutCortexTree,
} from "./cortex-draw";
import { cortexTreeFromReading } from "./cortex-tree";

const branch = (
	id: number,
	parentId: number,
	token: string,
	depth: number,
	probability: number,
	prefix?: string,
) => ({
	id,
	parentId,
	token,
	prefix: prefix ?? (depth === 0 ? "" : token),
	depth,
	probability,
	count: 1,
});

const readingWith = (
	branches: ReturnType<typeof branch>[],
	sequence = "beta",
	maxHops = 1,
) => ({
	beamWidth: 2,
	maxHops,
	nodeCount: branches.length,
	sequence,
	branches,
	beams: [{ sequence: "beta", score: -0.1 }],
	classes: [{ name: "sell", probability: 0.9 }],
});

describe("cortexTreeFromReading", () => {
	it("highlights the MAP beam path instead of the sealed observation bag", () => {
		const tree = cortexTreeFromReading({
			...readingWith(
				[
					branch(0, -1, "•", 0, 1),
					branch(1, 0, "alpha", 1, 0.5, "alpha"),
					branch(2, 0, "beta", 1, 0.5, "beta"),
					branch(3, 1, "gamma", 2, 0.5, "alpha_gamma"),
				],
				"alpha_gamma_long_sealed_tail",
				3,
			),
			beams: [{ sequence: "beta", score: -0.2 }],
		});

		expect(tree?.beamPrefixes.has("beta")).toBe(true);
		expect(tree?.beamPrefixes.has("alpha")).toBe(false);
		expect(tree?.beamPrefixes.has("alpha_gamma")).toBe(false);
	});

	it("keeps sibling order token-stable when probabilities reorder", () => {
		const first = cortexTreeFromReading(
			readingWith([
				branch(0, -1, "•", 0, 1),
				branch(1, 0, "alpha", 1, 0.9),
				branch(2, 0, "beta", 1, 0.1),
			]),
		);
		const second = cortexTreeFromReading(
			readingWith([
				branch(0, -1, "•", 0, 1),
				branch(1, 0, "alpha", 1, 0.05),
				branch(2, 0, "beta", 1, 0.95),
			]),
		);

		expect(first?.root.children.map((node) => node.token)).toEqual([
			"alpha",
			"beta",
		]);
		expect(second?.root.children.map((node) => node.token)).toEqual([
			"alpha",
			"beta",
		]);
	});

	it("layouts from actual tree depth so shallow trees fill the canvas width", () => {
		const tree = cortexTreeFromReading(
			readingWith(
				[
					branch(0, -1, "•", 0, 1),
					branch(1, 0, "alpha", 1, 0.5, "alpha"),
					branch(2, 0, "beta", 1, 0.5, "beta"),
				],
				"alpha",
				23,
			),
		);

		expect(tree?.maxDepth).toBe(1);

		if (tree === null) {
			return;
		}

		const layout = layoutCortexTree(tree, 800, 400);
		const leafX = layout.xByID.get(1);

		expect(leafX).toBeCloseTo(74 + (800 - 74 - 116), 5);
	});

	it("keeps leaf ranks stable when a side branch appears", () => {
		const roster = new CortexLeafRoster();
		const first = cortexTreeFromReading(
			readingWith([
				branch(0, -1, "•", 0, 1),
				branch(1, 0, "alpha", 1, 0.5, "alpha"),
				branch(2, 0, "beta", 1, 0.5, "beta"),
			]),
		);
		const second = cortexTreeFromReading(
			readingWith([
				branch(0, -1, "•", 0, 1),
				branch(1, 0, "alpha", 1, 0.4, "alpha"),
				branch(2, 0, "beta", 1, 0.4, "beta"),
				branch(3, 0, "gamma", 1, 0.2, "gamma"),
			]),
		);

		expect(first).not.toBeNull();
		expect(second).not.toBeNull();

		if (first === null || second === null) {
			return;
		}

		const before = layoutCortexTree(first, 800, 400, roster);
		const after = layoutCortexTree(second, 800, 400, roster);

		expect(after.yByID.get(1)).toBe(before.yByID.get(1));
		expect(after.yByID.get(2)).toBe(before.yByID.get(2));
	});
});

describe("displayToken", () => {
	it("shortens polarity metrics to mockup-scale labels", () => {
		expect(displayToken("cvd-absorption--positive")).toBe("absorption+");
		expect(displayToken("depthflow-thin-score--negative")).toBe("score-");
		expect(displayToken("sweep")).toBe("sweep");
	});

	it("keeps symbol tokens readable instead of collapsing to the quote", () => {
		expect(displayToken("symbol-btc-usd")).toBe("BTC/USD");
		expect(displayToken("symbol-cloud-usd")).toBe("CLOUD/USD");
	});
});
