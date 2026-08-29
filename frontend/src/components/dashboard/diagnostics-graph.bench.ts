import { bench, describe } from "vitest";
import type { EdgeStats, NodeStats } from "#/collections/topology";
import { buildDiagnosticsGraph } from "./diagnostics-graph";

const node = (label: string, seqCount: number): NodeStats => ({
	label,
	seqCount,
	avgGapNs: 8_000_000,
	lastGapNs: 8_100_000,
	lastAtNs: 10_000_000_000,
	backlog: 0,
	maxBacklog: 0,
});

const edge = (from: string, to: string): EdgeStats => ({
	from,
	to,
	hopCount: 10_000,
	avgLatencyNs: 42_000,
	lastLatencyNs: 40_000,
	lastAtNs: 10_000_000_000,
});

const LABELS = [
	"ticker.ingress",
	"ticker.signals",
	"ticker.category",
	"ticker.hub",
	"trade.ingress",
	"trade.signals",
	"trade.hub",
	"level3.ingress",
	"level3.signals",
	"level3.hub",
];

const HOPS: [string, string][] = [
	["ticker.ingress", "ticker.signals"],
	["ticker.signals", "ticker.category"],
	["ticker.category", "ticker.hub"],
	["trade.ingress", "trade.signals"],
	["trade.signals", "trade.hub"],
	["level3.ingress", "level3.signals"],
	["level3.signals", "level3.hub"],
];

const NODES = new Map(LABELS.map((label, index) => [label, node(label, 10_000 + index)]));
const EDGES = new Map(HOPS.map(([from, to]) => [`${from}>${to}`, edge(from, to)]));

describe("buildDiagnosticsGraph", () => {
	bench("lays out and routes the discovered topology", () => {
		buildDiagnosticsGraph(NODES, EDGES);
	});
});
