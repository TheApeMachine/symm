import { createStore } from "@tanstack/react-store";
import type { BoundaryStamp } from "#/providers/telemetry/telemetry/boundary-stamp";
import type { Measurement, MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";

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
	sentiment: { group: "signal", stage: 1 },
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
	if (base === "spot" || base === "trade" || base === "ticker" || base === "book") {
		return "kraken";
	}
	return base;
};

type ExtractedMeasurement = {
	source: string;
	at: number;
	backlog: number;
	group?: string;
	stage?: number;
	peers: { source: string; at: number }[];
	rawMeasurement?: MeasurementT;
};

function extractMeasurement(
	item: Measurement | MeasurementT,
): ExtractedMeasurement | null {
	if (!item) return null;

	if ("unpack" in item && typeof (item as Measurement).source === "function") {
		const m = item as Measurement;
		const rawSource = m.source();
		const source = normalizeSource(rawSource);
		if (!source) return null;

		const at = Number(m.at());
		let backlog = 0;
		const metricsLen = m.metricsLength();
		for (let i = 0; i < metricsLen; i++) {
			const met = m.metrics(i);
			if (met && met.name() === "backlog") {
				backlog = Number(met.raw());
				break;
			}
		}

		let group: string | undefined;
		let stage: number | undefined;

		const provLen = m.provenanceLength();
		for (let i = 0; i < provLen; i++) {
			const entry = m.provenance(i);
			if (!entry) continue;
			if (entry.name() === "group") {
				const val = entry.value();
				group = typeof val === "string" ? val : undefined;
			}
		}

		const metaLen = m.metadataLength();
		for (let i = 0; i < metaLen; i++) {
			const entry = m.metadata(i);
			if (!entry) continue;
			if (entry.name() === "group" && !group) {
				group = String(entry.value());
			}
			if (entry.name() === "stage") {
				stage = Number(entry.value());
			}
		}

		const peers: { source: string; at: number }[] = [];
		const peersLen = m.peersLength();

		for (let i = 0; i < peersLen; i++) {
			const peer = m.peers(i);
			if (peer) {
				const peerSource = normalizeSource(peer.source());
				if (peerSource && peerSource !== source) {
					peers.push({
						source: peerSource,
						at: Number(peer.at()),
					});
				}
			}
		}

		const rawMeasurement =
			typeof m.unpack === "function" ? m.unpack() : undefined;

		return { source, at, backlog, group, stage, peers, rawMeasurement };
	}

	const mt = item as MeasurementT;
	const source = normalizeSource(mt.source);
	if (!source) return null;

	const at = Number(mt.at ?? 0n);
	let backlog = 0;
	for (const met of mt.metrics ?? []) {
		if (met?.name === "backlog") {
			backlog = Number(met.raw);
			break;
		}
	}

	let group: string | undefined;
	let stage: number | undefined;

	for (const entry of mt.provenance ?? []) {
		if (!entry) continue;
		if (entry.name === "group") {
			const val = entry.value;
			group =
				typeof val === "string"
					? val
					: val
						? new TextDecoder().decode(val)
						: undefined;
		}
	}

	for (const entry of mt.metadata ?? []) {
		if (!entry) continue;
		if (entry.name === "group" && !group) {
			group = String(entry.value);
		}
		if (entry.name === "stage") {
			stage = Number(entry.value);
		}
	}

	const peers: { source: string; at: number }[] = [];

	for (const peer of mt.peers ?? []) {
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

	return { source, at, backlog, group, stage, peers, rawMeasurement: mt };
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
	lastMeasurement?: MeasurementT;
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
					version: prev.version + 1,
				};
			});
		},

		ingestMeasurements: (
			target: MeasurementsFrame | (Measurement | MeasurementT)[],
		) => {
			const extracted: ExtractedMeasurement[] = [];

			if ("rowsLength" in target && typeof target.rowsLength === "function") {
				const count = target.rowsLength();
				for (let i = 0; i < count; i++) {
					const row = target.rows(i);
					if (row) {
						const ex = extractMeasurement(row);
						if (ex) extracted.push(ex);
					}
				}
			} else if (Array.isArray(target)) {
				for (const item of target) {
					const ex = extractMeasurement(item);
					if (ex) extracted.push(ex);
				}
			}

			if (extracted.length === 0) return;

			setState((prev) => {
				for (const measurement of extracted) {
					const { source, peers, backlog, group: metaGroup, stage: metaStage } = measurement;
					const defaultTopo = PIPELINE_TOPOLOGY[source] ?? {
						group: "signal",
						stage: 1,
					};
					const group = metaGroup ?? defaultTopo.group;
					const stage = metaStage ?? defaultTopo.stage;

					const atNs =
						measurement.at > 0 ? measurement.at : Date.now() * 1_000_000;
					const existingNode = prev.nodes.get(source);

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
						lastMeasurement:
							measurement.rawMeasurement ?? existingNode?.lastMeasurement,
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
						const latencyNs =
							p.at > 0 && atNs > p.at ? atNs - p.at : 0;
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
					version: prev.version + 1,
				};
			});
		},
	}),
);
