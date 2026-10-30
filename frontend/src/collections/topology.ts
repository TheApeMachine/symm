import { createStore } from "@tanstack/react-store";

export interface BoundaryStamp {
	label(): string | null;
	group(): string | null;
	stage(): number;
	atNs(): bigint;
	seqCount(): bigint;
	avgGapNs(): bigint;
	lastGapNs(): bigint;
	backlog(): bigint;
}

export const PIPELINE_TOPOLOGY: Record<
	string,
	{ group: string; stage: number }
> = {
	kraken: { group: "venue", stage: 0 },
	spot: { group: "venue", stage: 0 },
	trade: { group: "venue", stage: 0 },
	ticker: { group: "venue", stage: 0 },
	book: { group: "venue", stage: 0 },

	correlation: { group: "signal", stage: 1 },
	cvd: { group: "signal", stage: 1 },
	depthflow: { group: "signal", stage: 1 },
	hawkes: { group: "signal", stage: 1 },
	leadlag: { group: "signal", stage: 1 },
	liquidity: { group: "signal", stage: 1 },
	morphology: { group: "signal", stage: 1 },
	pumpdump: { group: "signal", stage: 1 },
	toxicity: { group: "signal", stage: 1 },

	manifold: { group: "logic", stage: 2 },
	resonance: { group: "logic", stage: 2 },
	cognition: { group: "logic", stage: 2 },

	training: { group: "strategy", stage: 3 },
	paper: { group: "strategy", stage: 3 },
	impulse: { group: "strategy", stage: 3 },
	trader: { group: "strategy", stage: 3 },

	ui_tee: { group: "tees", stage: 4 },
	store_tee: { group: "tees", stage: 4 },
};

export const normalizeSource = (
	raw: string | Uint8Array | null | undefined,
): string => {
	if (!raw) return "";
	const text = typeof raw === "string" ? raw : new TextDecoder().decode(raw);
	const lower = text.toLowerCase().trim();
	const base = lower.includes(":") ? lower.split(":")[0] : lower;
	if (
		base === "spot" ||
		base === "trade" ||
		base === "ticker" ||
		base === "book"
	) {
		return "kraken";
	}
	return base;
};

/*
TopologyValue is the slice ingestValues actually reads. Live frames are Value
instances; tests and older frames pass the same fields as plain objects, and
peers only exist on that wider shape.
*/
export type TopologyPeer = {
	id?: string;
	source?: string | Uint8Array | null;
	at?: bigint | number;
};

export type TopologyMetric = {
	name?: string;
	raw?: number;
	snm?: number;
};

export type TopologyTag = {
	name?: string;
	value?: string | number;
};

export type TopologyValue = {
	id?: string;
	tick?: bigint;
	timestamp?: bigint;
	origin?: string;
	scope?: string;
	source?: string | Uint8Array | null;
	at?: bigint | number;
	metric?: (name: string) => number | undefined;
	metrics?: TopologyMetric[];
	metadata?: TopologyTag[];
	provenance?: TopologyTag[];
	peers?: TopologyPeer[];
};

type ExtractedValue = {
	source: string;
	at: number;
	backlog: number;
	group?: string;
	stage?: number;
	peers: { source: string; at: number }[];
	rawValue?: TopologyValue;
};

function extractValue(
	item: TopologyValue | null | undefined,
): ExtractedValue | null {
	if (!item) return null;

	const rawSource =
		(typeof item.source === "string" ? item.source : item.origin) || "";
	const source = normalizeSource(rawSource);
	if (!source) return null;

	// `at` is a processing stamp. The value's timestamp is when the tick was
	// built, which is the same instant for every layer of that tick, so using
	// it as a clock reports the gap between trades as if it were stage cost.
	const at = Number(item.at ?? 0);
	let backlog = 0;
	if (typeof item.metric === "function") {
		backlog = item.metric("backlog") ?? 0;
	} else if (Array.isArray(item.metrics)) {
		for (const m of item.metrics) {
			if (m?.name === "backlog" && typeof m.raw === "number") {
				backlog = m.raw;
				break;
			}
		}
	}

	let group: string | undefined;
	let stage: number | undefined;

	if (Array.isArray(item.provenance)) {
		for (const entry of item.provenance) {
			if (!entry) continue;
			if (entry.name === "group") group = String(entry.value);
			if (entry.name === "stage") stage = Number(entry.value);
		}
	}

	if (Array.isArray(item.metadata)) {
		for (const entry of item.metadata) {
			if (!entry) continue;
			if (entry.name === "group" && !group) group = String(entry.value);
			if (entry.name === "stage" && stage === undefined)
				stage = Number(entry.value);
		}
	}

	const peers: { source: string; at: number }[] = [];
	if (Array.isArray(item.peers)) {
		for (const peer of item.peers) {
			if (peer) {
				const peerSource = normalizeSource(peer.source);
				if (peerSource && peerSource !== source) {
					peers.push({
						source: peerSource,
						at: Number(peer.at ?? 0n),
					});
				}
			}
		}
	}

	return { source, at, backlog, group, stage, peers, rawValue: item };
}

/*
NodeStats is one diagnostics stage's live health, read straight off its own
latest boundary stamp — no recomputation from a raw trace, since
system.Diagnostic (Go) already carries a running summary on every stamp.
*/
export type NodeStats = {
	label: string;
	// group is the runtime ring this stage runs in and stage its handler-group
	// index within that ring, both reported by the Workload that owns it (see
	// runtime.Composed). This is the only trustworthy grouping available: the
	// nodes of one handler group run concurrently against the same envelope,
	// so their stamps arrive in goroutine-completion order and the "edges"
	// between siblings are a race, not a hop.
	group: string;
	stage: number;
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
	// Work is time inside the stage. Wait is time it sat blocked before that
	// work: the ring queue, or the previous stage's barrier. These come from
	// the workspace trace, not from the gap between tick timestamps.
	lastWorkNs?: number;
	avgWorkNs?: number;
	maxWorkNs?: number;
	lastWaitNs?: number;
	avgWaitNs?: number;
	maxWaitNs?: number;
	// side is a measurement that shares the machine but is not on the tick's
	// critical path. The book apply is one: it runs on its own ingress.
	side?: boolean;
	lastValue?: TopologyValue | null;
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

/*
PipelineAccount is the last measured tick: where the time went, which stage
burned the most of it, and which one sat blocked the longest.
*/
export type PipelineAccount = {
	tick: number;
	bytes: number;
	drops: number;
	criticalNs: number;
	// cadenceNs is how often a whole tick arrives. It is one number for the
	// pipeline: every stage of a tick is measured in the same trace, so copying
	// it onto each stage reported one rate ten times.
	cadenceNs: number;
	bottleneck: string;
	bottleneckNs: number;
	stalled: string;
	stalledNs: number;
};

export type TraceSpan = {
	name: string;
	group: string;
	stage: number;
	waitNs: number;
	workNs: number;
	backlog: number;
	side?: boolean;
};

export type TraceFrame = {
	tick: number;
	seq: number;
	bytes: number;
	drops: number;
	spans: TraceSpan[];
};

// The order a trade tick actually walks. Venue is not in it: the book apply
// is a side measurement and only feeds the assemble step.
const FLOW_GROUPS = ["ingress", "signal", "strategy", "sink"];

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
	account?: PipelineAccount;
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
		of DISTINCT handler groups refreshes (or creates) the edge between them.

		Stages inside one handler group run concurrently against the same
		envelope, so their stamps arrive in goroutine-completion order. Pairing
		them would mint an edge in whichever direction won that race, and over
		many envelopes every ordering gets observed at least once — the group
		converges to a fully-connected mesh that never existed. So a run of
		same-group stamps is collapsed: every stage of the previous group is
		wired to every stage of the next, which is the real fan-out, and no
		edge is ever drawn between siblings.

		O(stamps) per envelope plus the group fan-out, no allocation beyond the
		occasional new Map entry for a label/hop seen for the first time.
		*/
		ingest: (stamps: BoundaryStamp[]) => {
			if (stamps.length === 0) return;

			setState((prev) => {
				for (const stamp of stamps) {
					const label = stamp.label() ?? "";
					if (!label) continue;

					const backlog = Number(stamp.backlog());
					const previousMax = prev.nodes.get(label)?.maxBacklog ?? 0;

					prev.nodes.set(label, {
						label,
						group: stamp.group() ?? "",
						stage: stamp.stage(),
						seqCount: Number(stamp.seqCount()),
						avgGapNs: Number(stamp.avgGapNs()),
						lastGapNs: Number(stamp.lastGapNs()),
						lastAtNs: Number(stamp.atNs()),
						backlog,
						maxBacklog: Math.max(previousMax, backlog),
					});
				}

				// Collapse the trace into runs of concurrent siblings, so the
				// wiring below joins groups rather than racing goroutines.
				const runs: {
					labels: string[];
					atNs: bigint;
					group: string;
					stage: number;
				}[] = [];

				for (const stamp of stamps) {
					const label = stamp.label() ?? "";
					if (!label) continue;

					const group = stamp.group() ?? "";
					const previous = runs[runs.length - 1];
					const sibling =
						previous !== undefined &&
						group !== "" &&
						group === previous.group &&
						stamp.stage() === previous.stage;

					if (sibling) {
						previous.labels.push(label);
						// The group is only complete once its slowest sibling
						// lands, so the run carries that stamp's time.
						previous.atNs = stamp.atNs();
						continue;
					}

					runs.push({
						labels: [label],
						atNs: stamp.atNs(),
						group,
						stage: stamp.stage(),
					});
				}

				for (let i = 1; i < runs.length; i++) {
					const from = runs[i - 1];
					const to = runs[i];
					const latencyNs = Number(to.atNs - from.atNs);

					for (const fromLabel of from.labels) {
						for (const toLabel of to.labels) {
							const key = edgeKey(fromLabel, toLabel);
							const existing = prev.edges.get(key);

							if (!existing) {
								prev.edges.set(key, {
									from: fromLabel,
									to: toLabel,
									hopCount: 1,
									avgLatencyNs: latencyNs,
									lastLatencyNs: latencyNs,
									lastAtNs: Number(to.atNs),
								});
								continue;
							}

							existing.hopCount += 1;
							existing.avgLatencyNs +=
								(latencyNs - existing.avgLatencyNs) * EDGE_LATENCY_EMA_WEIGHT;
							existing.lastLatencyNs = latencyNs;
							existing.lastAtNs = Number(to.atNs);
						}
					}
				}

				return {
					nodes: prev.nodes,
					edges: prev.edges,
					account: prev.account,
					version: prev.version + 1,
				};
			});
		},

		ingestValues: (target: readonly TopologyValue[]) => {
			const extracted: ExtractedValue[] = [];

			for (const item of target) {
				const ex = extractValue(item);
				if (ex) extracted.push(ex);
			}

			if (extracted.length === 0) return;

			setState((prev) => {
				for (const val of extracted) {
					const {
						source,
						peers,
						backlog,
						group: metaGroup,
						stage: metaStage,
					} = val;
					const defaultTopo = PIPELINE_TOPOLOGY[source] ?? {
						group: "signal",
						stage: 1,
					};
					const group = metaGroup ?? defaultTopo.group;
					const stage = metaStage ?? defaultTopo.stage;

					const atNs = val.at > 0 ? val.at : 0;
					const existingNode = prev.nodes.get(source);

					// A live value carries the stage's last reading. It does
					// not carry how long the stage took; the trace does. Leave
					// the timings already measured on the node alone.
					if (atNs <= 0) {
						const kept = existingNode ?? {
							label: source,
							group,
							stage,
							seqCount: 0,
							avgGapNs: 0,
							lastGapNs: 0,
							lastAtNs: 0,
							backlog: 0,
							maxBacklog: 0,
						};
						prev.nodes.set(source, {
							...kept,
							group: metaGroup ?? kept.group,
							stage: metaStage ?? kept.stage,
							backlog: backlog || kept.backlog,
							maxBacklog: Math.max(kept.maxBacklog, backlog),
							lastValue: val.rawValue ?? kept.lastValue,
						});
						continue;
					}

					let seqCount = 1;
					let avgGapNs = 0;
					let lastGapNs = 0;
					let maxBacklog = backlog;

					if (existingNode) {
						seqCount = existingNode.seqCount + 1;
						maxBacklog = Math.max(existingNode.maxBacklog, backlog);
						if (existingNode.lastAtNs > 0 && atNs > existingNode.lastAtNs) {
							lastGapNs = atNs - existingNode.lastAtNs;
							avgGapNs =
								existingNode.avgGapNs <= 0
									? lastGapNs
									: existingNode.avgGapNs +
										(lastGapNs - existingNode.avgGapNs) *
											EDGE_LATENCY_EMA_WEIGHT;
						} else {
							lastGapNs = existingNode.lastGapNs;
							avgGapNs = existingNode.avgGapNs;
						}
					}

					const activeValue = val.rawValue ?? existingNode?.lastValue;
					prev.nodes.set(source, {
						label: source,
						group,
						stage,
						seqCount,
						avgGapNs,
						lastGapNs,
						lastAtNs: atNs,
						backlog,
						maxBacklog,
						lastValue: activeValue,
					});

					for (const p of peers) {
						const feeder = p.source;
						if (!feeder || feeder === source) continue;

						if (!prev.nodes.has(feeder)) {
							const feederTopo = PIPELINE_TOPOLOGY[feeder] ?? {
								group: stage > 1 ? "signal" : "venue",
								stage: Math.max(0, stage - 1),
							};
							prev.nodes.set(feeder, {
								label: feeder,
								group: feederTopo.group,
								stage: feederTopo.stage,
								seqCount: 0,
								avgGapNs: 0,
								lastGapNs: 0,
								lastAtNs: 0,
								backlog: 0,
								maxBacklog: 0,
							});
						}

						const key = edgeKey(feeder, source);
						const latencyNs = p.at > 0 && atNs > p.at ? atNs - p.at : 0;
						const existingEdge = prev.edges.get(key);

						if (!existingEdge) {
							prev.edges.set(key, {
								from: feeder,
								to: source,
								hopCount: 1,
								avgLatencyNs: latencyNs,
								lastLatencyNs: latencyNs,
								lastAtNs: atNs,
							});
						} else {
							existingEdge.hopCount += 1;
							if (latencyNs > 0) {
								existingEdge.avgLatencyNs =
									existingEdge.avgLatencyNs <= 0
										? latencyNs
										: existingEdge.avgLatencyNs +
											(latencyNs - existingEdge.avgLatencyNs) *
												EDGE_LATENCY_EMA_WEIGHT;
								existingEdge.lastLatencyNs = latencyNs;
							}
							existingEdge.lastAtNs = atNs;
						}
					}
				}

				return {
					nodes: prev.nodes,
					edges: prev.edges,
					account: prev.account,
					version: prev.version + 1,
				};
			});
		},

		/*
		ingestTrace folds one workspace measurement into the graph. Edges are
		the rings the tick actually walked — ingress, then the signal ring,
		then strategy, then the sinks — not the order goroutines happened to
		finish. A stage's cost is its own work; the wire into it carries that
		same number so the dashed flow and the card agree.
		*/
		ingestTrace: (frame: TraceFrame) => {
			if (!frame || !Array.isArray(frame.spans) || frame.spans.length === 0) {
				return;
			}

			setState((prev) => {
				const now = Date.now() * 1_000_000;
				const seen = new Set<string>();
				const priorAt = prev.account
					? [...prev.nodes.values()].reduce(
							(latest, node) => Math.max(latest, node.lastAtNs),
							0,
						)
					: 0;
				const gap = priorAt > 0 && now > priorAt ? now - priorAt : 0;
				const prevCadence = prev.account?.cadenceNs ?? 0;
				const cadenceNs =
					gap <= 0
						? prevCadence
						: prevCadence <= 0
							? gap
							: prevCadence + (gap - prevCadence) * EDGE_LATENCY_EMA_WEIGHT;

				for (const span of frame.spans) {
					if (!span?.name) continue;

					seen.add(span.name);
					const existing = prev.nodes.get(span.name);
					const work = Math.max(0, span.workNs);
					const wait = Math.max(0, span.waitNs);
					const backlog = Math.max(0, span.backlog);
					let seqCount = 1;
					let avgWorkNs = work;
					let avgWaitNs = wait;

					if (existing) {
						seqCount = existing.seqCount + 1;

						const prevWork = existing.avgWorkNs ?? 0;
						const prevWait = existing.avgWaitNs ?? 0;
						avgWorkNs =
							prevWork <= 0
								? work
								: prevWork + (work - prevWork) * EDGE_LATENCY_EMA_WEIGHT;
						avgWaitNs =
							prevWait <= 0
								? wait
								: prevWait + (wait - prevWait) * EDGE_LATENCY_EMA_WEIGHT;
					}

					prev.nodes.set(span.name, {
						label: span.name,
						group: span.group,
						stage: span.stage,
						seqCount,
						avgGapNs: 0,
						lastGapNs: 0,
						lastAtNs: now,
						backlog,
						maxBacklog: Math.max(existing?.maxBacklog ?? 0, backlog),
						lastWorkNs: work,
						avgWorkNs,
						maxWorkNs: Math.max(existing?.maxWorkNs ?? 0, work),
						lastWaitNs: wait,
						avgWaitNs,
						maxWaitNs: Math.max(existing?.maxWaitNs ?? 0, wait),
						side: span.side === true,
						lastValue: existing?.lastValue,
					});
				}

				for (const label of [...prev.nodes.keys()]) {
					if (!seen.has(label)) prev.nodes.delete(label);
				}

				const byGroup = new Map<string, NodeStats[]>();

				for (const node of prev.nodes.values()) {
					const list = byGroup.get(node.group) ?? [];
					list.push(node);
					byGroup.set(node.group, list);
				}

				const wanted = new Map<
					string,
					{ from: string; to: string; work: number }
				>();
				const link = (
					sources: NodeStats[] | undefined,
					destinations: NodeStats[] | undefined,
				) => {
					if (sources === undefined || destinations === undefined) return;

					const orderedSources = [...sources].sort((a, b) =>
						a.label.localeCompare(b.label),
					);
					const orderedDestinations = [...destinations].sort((a, b) =>
						a.label.localeCompare(b.label),
					);

					for (const source of orderedSources) {
						for (const destination of orderedDestinations) {
							if (source.label === destination.label) continue;

							wanted.set(edgeKey(source.label, destination.label), {
								from: source.label,
								to: destination.label,
								work: destination.lastWorkNs ?? 0,
							});
						}
					}
				};

				link(byGroup.get("venue"), byGroup.get("ingress"));

				for (let index = 1; index < FLOW_GROUPS.length; index++) {
					link(
						byGroup.get(FLOW_GROUPS[index - 1]),
						byGroup.get(FLOW_GROUPS[index]),
					);
				}

				for (const key of [...prev.edges.keys()]) {
					if (!wanted.has(key)) prev.edges.delete(key);
				}

				for (const [key, hop] of wanted) {
					const existing = prev.edges.get(key);

					if (!existing) {
						prev.edges.set(key, {
							from: hop.from,
							to: hop.to,
							hopCount: 1,
							avgLatencyNs: hop.work,
							lastLatencyNs: hop.work,
							lastAtNs: now,
						});
						continue;
					}

					existing.hopCount += 1;
					existing.avgLatencyNs =
						existing.avgLatencyNs <= 0
							? hop.work
							: existing.avgLatencyNs +
								(hop.work - existing.avgLatencyNs) * EDGE_LATENCY_EMA_WEIGHT;
					existing.lastLatencyNs = hop.work;
					existing.lastAtNs = now;
				}

				let criticalNs = 0;
				let bottleneck = "";
				let bottleneckNs = 0;
				let stalled = "";
				let stalledNs = 0;

				for (const group of FLOW_GROUPS) {
					let barrier = 0;

					for (const span of frame.spans) {
						if (span.group !== group || span.side) continue;

						const cost = Math.max(0, span.waitNs) + Math.max(0, span.workNs);

						if (cost > barrier) barrier = cost;

						if (span.workNs > bottleneckNs) {
							bottleneckNs = span.workNs;
							bottleneck = span.name;
						}

						if (span.waitNs > stalledNs) {
							stalledNs = span.waitNs;
							stalled = span.name;
						}
					}

					criticalNs += barrier;
				}

				return {
					nodes: prev.nodes,
					edges: prev.edges,
					account: {
						tick: frame.tick,
						bytes: frame.bytes,
						drops: frame.drops,
						criticalNs,
						cadenceNs,
						bottleneck,
						bottleneckNs,
						stalled: stalledNs > 0 ? stalled : "",
						stalledNs,
					},
					version: prev.version + 1,
				};
			});
		},
	}),
);
