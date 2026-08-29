import { createStore } from "@tanstack/react-store";
import type { EnvelopeBoundaryStamp } from "#/providers/telemetry/telemetry/envelope-boundary-stamp";

/*
NodeStats is one diagnostics stage's live health, read straight off its own
latest boundary stamp — no recomputation from a raw trace, since
system.Diagnostic (Go) already carries a running summary on every stamp.
*/
export type NodeStats = {
	label: string;
	seqCount: number;
	avgGapNs: number;
	lastGapNs: number;
	lastAtNs: number;
	// backlog is how many sequence numbers behind the Workload's producer
	// this stage was on its most recent stamp — real ring pressure (see
	// system.Diagnostic's StepBacklog), 0 when fully caught up.
	backlog: number;
	// maxBacklog is the highest backlog observed at this stage this session
	// — the "how close did this ring get to backing up" reading, since a
	// single instantaneous backlog reading can look fine right after a spike
	// drains.
	maxBacklog: number;
};

/*
EdgeStats is one observed hop between two consecutive stage labels. Edges are
never declared — they exist purely because some envelope's boundary trace
visited `from` immediately before `to`, so the topology this produces is
exactly the pipeline envelopes actually took, nothing hand-maintained.
*/
export type EdgeStats = {
	from: string;
	to: string;
	hopCount: number;
	avgLatencyNs: number;
	lastLatencyNs: number;
	lastAtNs: number;
};

// EMA smoothing weight for edge latency, matching the ~16-sample half-life
// system.Diagnostic uses server-side (avg += (gap-avg) >> 4). JS numbers are
// float64, not int64, so this uses the equivalent floating-point weight
// (1/16) rather than a bit-shift, which would silently truncate once a
// nanosecond delta exceeds 32 bits (~2.1s).
const EDGE_LATENCY_EMA_WEIGHT = 1 / 16;

// An edge/node not refreshed within this window is considered idle rather
// than actively flowing, for animation and "live" vs "stale" coloring.
export const TOPOLOGY_LIVE_WINDOW_NS = 2_000_000_000;

type TopologyState = {
	nodes: Map<string, NodeStats>;
	edges: Map<string, EdgeStats>;
	// version increments on every ingest so a shallow-equality selector
	// (useSelector reading the Map references) still notices updates without
	// cloning either Map on every single envelope.
	version: number;
};

const edgeKey = (from: string, to: string) => `${from}>${to}`;

const initialTopologyState: TopologyState = {
	nodes: new Map(),
	edges: new Map(),
	version: 0,
};

export const topologyStore = createStore(
	initialTopologyState,
	({ setState }) => ({
		/*
		ingest folds one envelope's ordered boundary trace into the running
		topology: each stamp refreshes its own node, and each consecutive pair
		refreshes (or creates) the edge between them. O(stamps) per envelope,
		no allocation beyond the occasional new Map entry for a label/hop seen
		for the first time.
		*/
		ingest: (stamps: EnvelopeBoundaryStamp[]) => {
			if (stamps.length === 0) return;

			setState((prev) => {
				for (const stamp of stamps) {
					const label = stamp.label() ?? "";
					if (!label) continue;

					const backlog = Number(stamp.backlog());
					const previousMax = prev.nodes.get(label)?.maxBacklog ?? 0;

					prev.nodes.set(label, {
						label,
						seqCount: Number(stamp.seqCount()),
						avgGapNs: Number(stamp.avgGapNs()),
						lastGapNs: Number(stamp.lastGapNs()),
						lastAtNs: Number(stamp.atNs()),
						backlog,
						maxBacklog: Math.max(previousMax, backlog),
					});
				}

				for (let i = 1; i < stamps.length; i++) {
					const fromLabel = stamps[i - 1].label() ?? "";
					const toLabel = stamps[i].label() ?? "";
					if (!fromLabel || !toLabel) continue;

					const latencyNs = Number(stamps[i].atNs() - stamps[i - 1].atNs());
					const key = edgeKey(fromLabel, toLabel);
					const existing = prev.edges.get(key);

					if (!existing) {
						prev.edges.set(key, {
							from: fromLabel,
							to: toLabel,
							hopCount: 1,
							avgLatencyNs: latencyNs,
							lastLatencyNs: latencyNs,
							lastAtNs: Number(stamps[i].atNs()),
						});
						continue;
					}

					existing.hopCount += 1;
					existing.avgLatencyNs +=
						(latencyNs - existing.avgLatencyNs) * EDGE_LATENCY_EMA_WEIGHT;
					existing.lastLatencyNs = latencyNs;
					existing.lastAtNs = Number(stamps[i].atNs());
				}

				return { nodes: prev.nodes, edges: prev.edges, version: prev.version + 1 };
			});
		},
	}),
);
