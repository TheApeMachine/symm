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

const node = (label: string, group = "", stage = 0): NodeStats => ({
	label,
	group,
	stage,
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
	const edges = new Map(
		pairs.map(([from, to]) => [`${from}>${to}`, edge(from, to)]),
	);

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
				Math.abs(a.from.y - b.from.y) < 0.01 &&
				Math.abs(a.to.y - b.to.y) < 0.01;
			if (!sameGap) continue;

			if ((a.from.x - b.from.x) * (a.to.x - b.to.x) < 0) crossings++;
		}
	}

	return crossings;
};

/*
The grouped fixture mirrors the live pipeline's real composition: ingress rings
that each run a fan-out of concurrent signals, feeding a logic ring and then a
strategy ring. Sibling stamps are included in both directions, because that is
exactly what a concurrent handler group produces — the stages race, and the
store records whichever finished first as a hop.
*/
const GROUPED: { label: string; group: string; stage: number }[] = [
	{ label: "ticker.ingress", group: "ticker", stage: 0 },
	{ label: "ticker.pumpdump", group: "ticker", stage: 1 },
	{ label: "ticker.leadlag", group: "ticker", stage: 1 },
	{ label: "ticker.liquidity", group: "ticker", stage: 1 },
	{ label: "trade.ingress", group: "trade", stage: 0 },
	{ label: "trade.cvd", group: "trade", stage: 1 },
	{ label: "trade.hawkes", group: "trade", stage: 1 },
	{ label: "logic.manifold", group: "logic", stage: 0 },
	{ label: "logic.cognition", group: "logic", stage: 1 },
	{ label: "strategy.planner", group: "strategy", stage: 0 },
	{ label: "strategy.hub", group: "strategy", stage: 1 },
];

const GROUPED_HOPS: [string, string][] = [
	["ticker.ingress", "ticker.pumpdump"],
	["ticker.ingress", "ticker.leadlag"],
	["ticker.ingress", "ticker.liquidity"],
	// The race between concurrent siblings, in both directions.
	["ticker.pumpdump", "ticker.leadlag"],
	["ticker.leadlag", "ticker.pumpdump"],
	["ticker.liquidity", "ticker.pumpdump"],
	["trade.ingress", "trade.cvd"],
	["trade.ingress", "trade.hawkes"],
	["trade.cvd", "trade.hawkes"],
	["trade.hawkes", "trade.cvd"],
	["ticker.pumpdump", "logic.manifold"],
	["trade.hawkes", "logic.manifold"],
	["logic.manifold", "logic.cognition"],
	["logic.cognition", "strategy.planner"],
	["strategy.planner", "strategy.hub"],
];

const groupedNodes = () =>
	new Map(
		GROUPED.map((entry) => [
			entry.label,
			node(entry.label, entry.group, entry.stage),
		]),
	);

const groupedEdges = () =>
	new Map(GROUPED_HOPS.map(([from, to]) => [`${from}>${to}`, edge(from, to)]));

const groupedGraph = () =>
	buildDiagnosticsGraph(groupedNodes(), groupedEdges());

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
					Math.abs(a.x - b.x) < NODE_HALF.w * 2 &&
					Math.abs(a.y - b.y) < NODE_HALF.h * 2;

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

	it("draws every stage of a ring inside that ring's own box", () => {
		const graph = groupedGraph();

		for (const group of graph.groups) {
			const stages = [...graph.placements.values()].filter(
				(placement) => placement.group === group.id,
			);

			expect(stages.length).toBeGreaterThan(0);

			for (const stage of stages) {
				expect(stage.x).toBeGreaterThanOrEqual(group.x);
				expect(stage.x).toBeLessThanOrEqual(group.x + group.w);
				expect(stage.y).toBeGreaterThanOrEqual(group.y + group.headerH);
				expect(stage.y).toBeLessThanOrEqual(group.y + group.h);
			}
		}
	});

	it("never overlaps two ring boxes", () => {
		const graph = groupedGraph();

		for (let i = 0; i < graph.groups.length; i++) {
			for (let j = i + 1; j < graph.groups.length; j++) {
				const a = graph.groups[i];
				const b = graph.groups[j];
				const overlaps =
					a.x < b.x + b.w &&
					b.x < a.x + a.w &&
					a.y < b.y + b.h &&
					b.y < a.y + a.h;

				expect(overlaps, `${a.id} overlaps ${b.id}`).toBe(false);
			}
		}
	});

	it("puts the ingress rings above the rings they feed", () => {
		const graph = groupedGraph();
		const box = (id: string) => graph.groups.find((group) => group.id === id);

		const ticker = box("ticker");
		const trade = box("trade");
		const logic = box("logic");
		const strategy = box("strategy");

		expect(ticker && trade && logic && strategy).toBeTruthy();
		if (!ticker || !trade || !logic || !strategy) return;

		// Ingress rings are the ones nothing feeds, so they land in the first
		// cluster layer — no rule names them as sources.
		expect(ticker.y).toBeLessThan(logic.y);
		expect(trade.y).toBeLessThan(logic.y);
		expect(logic.y).toBeLessThan(strategy.y);
	});

	it("keeps concurrent siblings on one row and drops the races between them", () => {
		const graph = groupedGraph();

		const siblings = ["ticker.pumpdump", "ticker.leadlag", "ticker.liquidity"]
			.map((id) => graph.placements.get(id))
			.filter(
				(placement): placement is NonNullable<typeof placement> =>
					placement !== undefined,
			);

		expect(siblings).toHaveLength(3);

		// One handler group, so one row: they run side by side against the
		// same envelope and nothing orders them.
		for (const sibling of siblings) {
			expect(sibling.y).toBeCloseTo(siblings[0].y, 6);
		}

		// Their stamps arrive in goroutine-completion order, which the store
		// records as hops in both directions. None of those are real.
		const races = graph.edges.filter(
			(e) =>
				e.from.startsWith("ticker.") &&
				e.to.startsWith("ticker.") &&
				e.from !== "ticker.ingress",
		);

		expect(races).toHaveLength(0);
	});

	it("packs a wide topology to the shape of the panel it has to fit", () => {
		// A panel three times as wide as it is tall, against a tall one.
		const wide = buildDiagnosticsGraph(groupedNodes(), groupedEdges(), {
			w: 1800,
			h: 600,
		});
		const tall = buildDiagnosticsGraph(groupedNodes(), groupedEdges(), {
			w: 600,
			h: 1000,
		});

		const shape = (graph: ReturnType<typeof buildDiagnosticsGraph>) =>
			graph.extent.w / graph.extent.h;

		expect(shape(wide)).toBeGreaterThan(shape(tall));
	});

	it("routes the whole grouped topology without cutting through a card", () => {
		const graph = groupedGraph();
		const cards = [...graph.placements.values()];

		for (const e of graph.edges) {
			for (let i = 1; i < e.points.length; i++) {
				const from = e.points[i - 1];
				const to = e.points[i];

				for (const card of cards) {
					if (card.id === e.from || card.id === e.to) continue;

					const hitsX =
						Math.max(from.x, to.x) > card.x - NODE_HALF.w &&
						Math.min(from.x, to.x) < card.x + NODE_HALF.w;
					const hitsY =
						Math.max(from.y, to.y) > card.y - NODE_HALF.h &&
						Math.min(from.y, to.y) < card.y + NODE_HALF.h;

					expect(
						hitsX && hitsY,
						`${e.from}→${e.to} cuts through ${card.id}`,
					).toBe(false);
				}
			}
		}
	});

	it("widens an outside detour with its lane rather than narrowing it", () => {
		// Every edge of a fan-out shares one channel, so they are handed lanes
		// either side of centre. A negative lane must still push a detour
		// further out: signed, it pulled the escape track back inside the very
		// column the detour exists to get around.
		const graph = graphOf([
			["hub", "a"],
			["hub", "b"],
			["hub", "c"],
			["hub", "d"],
			["a", "sink"],
			["b", "sink"],
			["c", "sink"],
			["d", "sink"],
			["hub", "sink"],
		]);

		const longHop = graph.edges.find(
			(e) => e.from === "hub" && e.to === "sink",
		);
		expect(longHop).toBeDefined();
		if (!longHop) return;

		const blockers = ["a", "b", "c", "d"]
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

				expect(hitsX && hitsY, `hub→sink cuts through ${blocker.id}`).toBe(
					false,
				);
			}
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
