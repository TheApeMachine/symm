export type PipelineNode = {
	id: string;
	group?: string;
	parent?: string;
	kind?: "group" | "sink" | string;
	n?: number;
	last_ns?: number;
	ema_ns?: number;
	seen_at?: number;
	status?: string;
	queue?: number;
};

export type PipelineEdge = {
	from: string;
	to: string;
};

export type PipelineFrame = {
	nodes: PipelineNode[];
	edges: PipelineEdge[];
	hub: string;
};

export const EMPTY_PIPELINE: PipelineFrame = {
	nodes: [],
	edges: [],
	hub: "idle",
};

export type HealthTone = "healthy" | "slight" | "high" | "error" | "idle" | "stale";

const CHEAP_NS = 200_000;
const STALE_NS = 5_000_000_000;
/*
healthOf colours a node from status, freshness, and share of the slowest peer.
*/
export const healthOf = (
	node: PipelineNode | undefined,
	peakEmaNs: number,
	nowNs = Date.now() * 1_000_000,
): HealthTone => {
	if (!node) return "idle";

	const status = (node.status ?? "").toLowerCase();
	if (status === "error" || status === "fatal") return "error";

	if (node.kind === "group") {
		return status === "ok" ? "healthy" : "idle";
	}

	if (!(node.n ?? 0) && node.queue === undefined) return "idle";

	const age =
		(node.seen_at ?? 0) > 0
			? nowNs - (node.seen_at ?? 0)
			: Number.POSITIVE_INFINITY;
	if (Number.isFinite(age) && age > STALE_NS) return "stale";

	const work = node.ema_ns ?? 0;
	if (work > 0 && work < CHEAP_NS) return "healthy";
	if (work <= 0) return "healthy";
	if (peakEmaNs <= 0 || work < peakEmaNs * 0.67 || work < 1_000_000) {
		return "slight";
	}

	return "high";
};

export const peakEma = (nodes: Iterable<PipelineNode>): number => {
	let peak = 0;
	for (const node of nodes) {
		if ((node.ema_ns ?? 0) > peak) peak = node.ema_ns ?? 0;
	}
	return peak;
};

export const formatNanos = (nanos: number | undefined): string => {
	if (nanos === undefined || !Number.isFinite(nanos) || nanos <= 0) return "—";
	if (nanos < 1_000) return `${nanos.toFixed(0)}ns`;
	if (nanos < 1_000_000) return `${(nanos / 1_000).toFixed(1)}µs`;
	if (nanos < 1_000_000_000) return `${(nanos / 1_000_000).toFixed(2)}ms`;
	return `${(nanos / 1_000_000_000).toFixed(2)}s`;
};

export const formatCount = (count: number): string =>
	new Intl.NumberFormat("en", { notation: "compact" }).format(count);

export const NODE_LABEL: Record<string, string> = {
	kraken: "KRAKEN",
	trade: "TRADE",
	level3: "LEVEL3",
	rest: "REST",
	ingress: "INGRESS",
	"ring-signal": "SIGNAL",
	"ring-resonance": "RESONANCE",
	"ring-model": "MODEL",
	"ring-persist": "PERSIST",
	"tick-persist": "TICK PERSIST",
	store: "STORE",
	s3: "S3",
	queue: "QUEUE",
	hub: "UI HUB",
	sequencer: "SEQUENCER",
	grid: "GRID",
	tree: "TREE",
	board: "BOARD",
};

export const labelOf = (id: string): string =>
	NODE_LABEL[id] ?? id.replace(/[_-]/g, " ").toUpperCase();

/*
mergeTopology applies a structural pipeline frame without wiping live metrics
already patched in by diag_pulse.
*/
export const mergeTopology = (
	prev: PipelineFrame,
	next: PipelineFrame,
): PipelineFrame => {
	const prior = new Map(
		(prev.nodes ?? []).map((node) => [node.id, node] as const),
	);

	const nodes = (next.nodes ?? []).map((node) => {
		const had = prior.get(node.id);
		if (!had) return node;

		return {
			...node,
			n: had.n ?? node.n,
			last_ns: had.last_ns ?? node.last_ns,
			ema_ns: had.ema_ns ?? node.ema_ns,
			seen_at: had.seen_at ?? node.seen_at,
			queue: had.queue ?? node.queue,
		};
	});

	return {
		nodes,
		edges: next.edges ?? prev.edges ?? [],
		hub: next.hub || prev.hub,
	};
};
