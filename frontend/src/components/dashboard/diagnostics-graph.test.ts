import { describe, expect, it } from "vitest";
import type { EdgeStats, NodeStats } from "#/collections/topology";
import { buildDiagnosticsGraph, HALF as NODE_HALF } from "./diagnostics-graph";

const edge = (from: string, to: string): EdgeStats => ({
	from,
	to,
	hopCount: 1,
	avgLatencyNs: 1_000,
	lastLatencyNs: 1_000,
	lastAtNs: 0,
});

const node = (label: string): NodeStats => ({
	label,
	seqCount: 1,
	avgGapNs: 1_000_000,
	lastGapNs: 1_000_000,
	lastAtNs: 0,
	backlog: 0,
	maxBacklog: 0,
});

const graphOf = (pairs: [string, string][]) => {
	const labels = new Set(pairs.flat());
	const nodes = new Map([...labels].map((label) => [label, node(label)]));
	const edges = new Map(pairs.map(([from, to]) => [`${from}>${to}`, edge(from, to)]));

	return buildDiagnosticsGraph(nodes, edges);
};

/*
countCrossings measures the rendered result the way a reader perceives it:
for every pair of edges that span the same vertical gap, their endpoints being
in opposite left-to-right order means the two wires visibly cross.
*/
const countCrossings = (graph: ReturnType<typeof graphOf>): number => {
	const spans = graph.edges
		.map((e) => {
			const from = graph.placements.get(e.from);
			const to = graph.placements.get(e.to);
			return from && to ? { from, to } : null;
		})
		.filter((s): s is NonNullable<typeof s> => s !== null);

	let crossings = 0;

	for (let i = 0; i < spans.length; i++) {
		for (let j = i + 1; j < spans.length; j++) {
			const a = spans[i];
			const b = spans[j];

			const sameGap =
				Math.abs(a.from.y - b.from.y) < 0.01 && Math.abs(a.to.y - b.to.y) < 0.01;
			if (!sameGap) continue;

			if ((a.from.x - b.from.x) * (a.to.x - b.to.x) < 0) crossings++;
		}
	}

	return crossings;
};

describe("buildDiagnosticsGraph layout", () => {
	it("untangles a deliberately crossed topology", () => {
		// Sources land in one layer, sinks in the next; the alphabetical
		// seeding order is the exact opposite of the connection order, so a
		// layout that does no crossing reduction fans every wire across every
		// other one.
		const graph = graphOf([
			["a1", "z9"],
			["a2", "z8"],
			["a3", "z7"],
			["a4", "z6"],
		]);

		expect(countCrossings(graph)).toBe(0);
	});

	it("keeps a fan-out from a single hub crossing-free", () => {
		const graph = graphOf([
			["ingest", "beta"],
			["ingest", "alpha"],
			["ingest", "gamma"],
			["beta", "sink"],
			["alpha", "sink"],
			["gamma", "sink"],
		]);

		expect(countCrossings(graph)).toBe(0);
	});

	it("layers a linear chain in pipeline order", () => {
		const graph = graphOf([
			["price", "desk"],
			["desk", "signals"],
			["signals", "logic"],
		]);

		const y = (id: string) => graph.placements.get(id)?.y ?? 0;

		expect(y("price")).toBeLessThan(y("desk"));
		expect(y("desk")).toBeLessThan(y("signals"));
		expect(y("signals")).toBeLessThan(y("logic"));
	});

	it("gives edges sharing a channel distinct lanes", () => {
		const graph = graphOf([
			["a1", "z1"],
			["a2", "z2"],
			["a3", "z3"],
		]);

		// Each route's mid-channel run should sit on its own track, so no two
		// edges traverse the gap along an identical line.
		const channels = graph.edges.map((e) =>
			e.points.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(" "),
		);

		expect(new Set(channels).size).toBe(channels.length);
	});

	it("never overlaps two node boxes, even at full topology size", () => {
		// 31 stages is the live pipeline's real size — the size at which
		// fraction-of-a-fixed-space placement collapsed boxes onto each other.
		const pairs: [string, string][] = [];
		for (let i = 0; i < 30; i++) {
			pairs.push([`stage${i}`, `stage${i + 1}`]);
		}
		for (let i = 0; i < 24; i++) {
			pairs.push([`stage${i}`, `stage${i + 3}`]);
		}

		const graph = graphOf(pairs);
		const boxes = [...graph.placements.values()];

		for (let i = 0; i < boxes.length; i++) {
			for (let j = i + 1; j < boxes.length; j++) {
				const a = boxes[i];
				const b = boxes[j];
				const overlaps =
					Math.abs(a.x - b.x) < NODE_HALF.w * 2 && Math.abs(a.y - b.y) < NODE_HALF.h * 2;

				expect(
					overlaps,
					`${a.id} overlaps ${b.id} at (${a.x},${a.y}) / (${b.x},${b.y})`,
				).toBe(false);
			}
		}
	});

	it("routes around nodes that sit between an edge's endpoints", () => {
		// A long hop that skips several layers: the direct line would plough
		// straight through the intervening stages.
		const graph = graphOf([
			["a", "b"],
			["b", "c"],
			["c", "d"],
			["d", "e"],
			["a", "e"],
		]);

		const longHop = graph.edges.find((e) => e.from === "a" && e.to === "e");
		expect(longHop).toBeDefined();
		if (!longHop) return;

		const blockers = ["b", "c", "d"]
			.map((id) => graph.placements.get(id))
			.filter((p): p is NonNullable<typeof p> => p !== undefined);

		for (let i = 1; i < longHop.points.length; i++) {
			const from = longHop.points[i - 1];
			const to = longHop.points[i];

			for (const blocker of blockers) {
				const hitsX =
					Math.max(from.x, to.x) > blocker.x - NODE_HALF.w &&
					Math.min(from.x, to.x) < blocker.x + NODE_HALF.w;
				const hitsY =
					Math.max(from.y, to.y) > blocker.y - NODE_HALF.h &&
					Math.min(from.y, to.y) < blocker.y + NODE_HALF.h;

				expect(hitsX && hitsY, `a→e cuts through ${blocker.id}`).toBe(false);
			}
		}
	});

	it("draws square corners", () => {
		const graph = graphOf([
			["a1", "z1"],
			["a2", "z1"],
		]);

		const bending = graph.edges.filter((e) => e.points.length > 3);
		expect(bending.length).toBeGreaterThan(0);

		// Straight orthogonal runs only — no curve commands of any kind.
		for (const e of graph.edges) {
			expect(e.d).not.toMatch(/[QCAST]/);
		}
	});

	it("reports an extent that contains every node", () => {
		const graph = graphOf([
			["a", "b"],
			["a", "c"],
			["b", "d"],
			["c", "d"],
		]);

		for (const placement of graph.placements.values()) {
			expect(placement.x).toBeGreaterThanOrEqual(graph.extent.x);
			expect(placement.x).toBeLessThanOrEqual(graph.extent.x + graph.extent.w);
			expect(placement.y).toBeGreaterThanOrEqual(graph.extent.y);
			expect(placement.y).toBeLessThanOrEqual(graph.extent.y + graph.extent.h);
		}
	});

	it("routes every edge without emitting a degenerate path", () => {
		const graph = graphOf([
			["a", "b"],
			["b", "c"],
			["a", "c"],
		]);

		for (const e of graph.edges) {
			expect(e.d).not.toContain("NaN");
			expect(e.points.length).toBeGreaterThanOrEqual(2);
		}
	});
});
