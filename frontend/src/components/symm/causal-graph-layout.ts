import type { CausalGraphRow } from "#/components/symm/causal-graph-data-provider";
import type { SimEdge, SimNode } from "#/components/symm/nodeModifiers";

export type CausalPeer = {
	symbol: string;
	correlation: number;
};

const DAG_NODES = [
	{ id: "macro", title: "Macro", geoX: -90, geoY: 70 },
	{ id: "liquidity", title: "Liquidity", geoX: 0, geoY: 100 },
	{ id: "flow", title: "Flow", geoX: 90, geoY: 70 },
	{ id: "velocity", title: "Velocity", geoX: 0, geoY: -80 },
] as const;

const PEARL_NODES = [
	{ id: "l1", title: "L1 Assoc", geoX: -50, geoY: -125 },
	{ id: "l2", title: "L2 Do", geoX: 0, geoY: -140 },
	{ id: "l3", title: "L3 CF", geoX: 50, geoY: -125 },
] as const;

const PEER_RING_RADIUS = 155;

export const formatGraphValue = (value: number): string => {
	if (!Number.isFinite(value) || value === 0) {
		return "0";
	}

	if (Math.abs(value) >= 1000) {
		return value.toExponential(2);
	}

	if (Math.abs(value) >= 1) {
		return value.toFixed(3);
	}

	return value.toFixed(4);
};

const peerShortLabel = (symbol: string): string =>
	symbol.split("/")[0] ?? symbol;

const peerPosition = (index: number, total: number) => {
	const angle = (index / Math.max(total, 1)) * Math.PI * 2 - Math.PI / 2;

	return {
		geoX: Math.cos(angle) * PEER_RING_RADIUS,
		geoY: Math.sin(angle) * PEER_RING_RADIUS * 0.55,
	};
};

const buildPeerNodes = (peers: CausalPeer[]): SimNode[] =>
	peers.map((peer, index) => {
		const position = peerPosition(index, peers.length);

		return {
			iata: peer.symbol,
			label: `${peerShortLabel(peer.symbol)} ${formatGraphValue(peer.correlation)}`,
			x: position.geoX,
			y: position.geoY,
			vx: 0,
			vy: 0,
			geoX: position.geoX,
			geoY: position.geoY,
		};
	});

const buildDagNodes = (): SimNode[] =>
	[...DAG_NODES, ...PEARL_NODES].map((node) => ({
		iata: node.id,
		label: node.title,
		x: node.geoX,
		y: node.geoY,
		vx: 0,
		vy: 0,
		geoX: node.geoX,
		geoY: node.geoY,
	}));

const buildDagEdges = (row: CausalGraphRow | undefined): SimEdge[] => {
	const backdoorWeight = Math.abs(row?.association ?? 0);

	return [
		{
			sourceIdx: 0,
			targetIdx: 3,
			weight: row?.coef_macro ?? 0,
			kind: "causal",
		},
		{
			sourceIdx: 2,
			targetIdx: 3,
			weight: row?.coef_flow ?? 0,
			kind: "causal",
		},
		{
			sourceIdx: 1,
			targetIdx: 3,
			weight: row?.coef_liquidity ?? 0,
			kind: "causal",
		},
		{
			sourceIdx: 1,
			targetIdx: 0,
			weight: backdoorWeight,
			kind: "backdoor",
		},
		{
			sourceIdx: 1,
			targetIdx: 2,
			weight: backdoorWeight,
			kind: "backdoor",
		},
		{
			sourceIdx: 3,
			targetIdx: 4,
			weight: row?.association ?? 0,
			kind: "ladder",
		},
		{
			sourceIdx: 4,
			targetIdx: 5,
			weight: row?.intervention ?? 0,
			kind: "ladder",
		},
		{
			sourceIdx: 5,
			targetIdx: 6,
			weight: row?.uplift ?? 0,
			kind: "ladder",
		},
	];
};

const buildPeerEdges = (peers: CausalPeer[], peerStartIdx: number): SimEdge[] =>
	peers.map((_peer, index) => ({
		sourceIdx: 0,
		targetIdx: peerStartIdx + index,
		weight: peers[index]?.correlation ?? 0,
		kind: "peer" as const,
	}));

export const applyGraphLabels = (
	nodes: SimNode[],
	row: CausalGraphRow | undefined,
) => {
	if (!row) {
		return;
	}

	const values: Record<string, number> = {
		macro: row.macro_momentum,
		liquidity: row.liquidity,
		flow: row.local_flow,
		velocity: row.price_velocity,
		l1: row.association,
		l2: row.intervention,
		l3: row.uplift,
	};

	for (const node of nodes) {
		const dagNode = [...DAG_NODES, ...PEARL_NODES].find(
			(entry) => entry.id === node.iata,
		);

		if (dagNode) {
			node.label = `${dagNode.title} ${formatGraphValue(values[dagNode.id] ?? 0)}`;
		}
	}
};

export const buildGraphState = (row: CausalGraphRow | undefined) => {
	const peers = row?.peers ?? [];
	const nodes = [...buildDagNodes(), ...buildPeerNodes(peers)];
	const peerStartIdx = DAG_NODES.length + PEARL_NODES.length;
	const edges = [...buildDagEdges(row), ...buildPeerEdges(peers, peerStartIdx)];

	applyGraphLabels(nodes, row);

	return { nodes, edges };
};

export const formatGraphCaption = (row: CausalGraphRow): string => {
	const ladder = `L1 ${formatGraphValue(row.association)} · L2 ${formatGraphValue(row.intervention)} · L3 ${formatGraphValue(row.uplift)}`;
	const confound =
		row.confounding_gap > 0
			? ` · gap ${formatGraphValue(row.confounding_gap)}`
			: "";
	const peers =
		row.peers && row.peers.length > 0
			? ` · peers ${row.peers.map((peer) => peerShortLabel(peer.symbol)).join(", ")}`
			: "";

	return `${row.reason || "warming"} · conf ${row.confidence.toFixed(3)} · n=${row.sample_count} · ${ladder}${confound}${peers}`;
};

export const replaceGraphNodes = (target: SimNode[], rebuilt: SimNode[]) => {
	target.length = 0;
	target.push(...rebuilt);
};
