import type { TraceNode } from "#/components/terminal/decision-trace-model";

/*
The War Room canvas: the causal MCTS search tree as the search actually built
it.

The geometry is the cortex tree's — depth on x, siblings averaged on y, bezier
edges — because the two are the same kind of object and an operator who has
learned to read one should not have to learn a second idiom. What differs is
what the ink means, and here it means search evidence:

  edge thickness   how many real rollouts travelled this branch
  amber path       the principal variation the search actually selected
  node fill        real rollout mass vs Pearl counterfactual mass, split
  hollow ring      a branch whose value is mostly imagined, not rolled out
  strike-through   causally rejected: the structural model condemned it

The split matters more than anything else drawn here. A branch that won because
Pearl's counterfactual backpropagation filled in its value is a different claim
from one that won by being rolled out twenty times, and a visualization that
blended them would hide exactly the thing worth auditing.
*/

export type WarRoomNode = {
	id: number;
	node: TraceNode;
	depth: number;
	parent: number;
	children: number[];
	/* onPath marks the principal variation the search selected. */
	onPath: boolean;
};

export type WarRoomTree = {
	nodes: WarRoomNode[];
	root: WarRoomNode | null;
	maxDepth: number;
	peakVisits: number;
	/* peakMass is the largest counterfactual mass on any node. */
	peakMass: number;
};

/*
flatten walks the trace tree into an indexable node list and marks the
principal variation.

The selected path is followed from the root through whichever child the search
marked selected, rather than recomputed from visit counts. The search already
made that choice under its own selection rule — including the causal bias — and
re-deriving it here from a different rule would draw a path the search never
took.
*/
export const warRoomTreeFrom = (root: TraceNode | null): WarRoomTree => {
	const nodes: WarRoomNode[] = [];

	if (!root) {
		return { nodes, root: null, maxDepth: 0, peakVisits: 0, peakMass: 0 };
	}

	let maxDepth = 0;
	let peakVisits = 0;
	let peakMass = 0;

	const walk = (source: TraceNode, depth: number, parent: number): number => {
		const id = nodes.length;

		const entry: WarRoomNode = {
			id,
			node: source,
			depth,
			parent,
			children: [],
			onPath: false,
		};

		nodes.push(entry);

		maxDepth = Math.max(maxDepth, depth);
		peakVisits = Math.max(peakVisits, source.visits);
		peakMass = Math.max(peakMass, source.counterfactualMass);

		for (const child of source.children) {
			entry.children.push(walk(child, depth + 1, id));
		}

		return id;
	};

	walk(root, 0, -1);

	// The principal variation: from the root, follow the child the search
	// marked selected for as long as one is marked.
	let cursor: WarRoomNode | undefined = nodes[0];

	while (cursor !== undefined) {
		cursor.onPath = true;

		const next: WarRoomNode | undefined = cursor.children
			.map((id) => nodes[id])
			.find((child) => child?.node.selected);

		cursor = next;
	}

	return { nodes, root: nodes[0] ?? null, maxDepth, peakVisits, peakMass };
};

export type WarRoomLayout = {
	x: Map<number, number>;
	y: Map<number, number>;
};

const PAD_LEFT = 62;
const PAD_RIGHT = 104;
const PAD_TOP = 26;
const PAD_BOTTOM = 20;

/*
layoutWarRoomTree places leaves evenly and averages each parent onto its
children, so a branch sits at the centre of what it leads to.
*/
export const layoutWarRoomTree = (
	tree: WarRoomTree,
	width: number,
	height: number,
): WarRoomLayout => {
	const x = new Map<number, number>();
	const y = new Map<number, number>();

	if (tree.root === null) {
		return { x, y };
	}

	const usableWidth = Math.max(1, width - PAD_LEFT - PAD_RIGHT);
	const usableHeight = Math.max(1, height - PAD_TOP - PAD_BOTTOM);
	const leaves: WarRoomNode[] = [];

	const collect = (entry: WarRoomNode): void => {
		if (entry.children.length === 0) {
			leaves.push(entry);

			return;
		}

		for (const id of entry.children) {
			const child = tree.nodes[id];

			if (child !== undefined) collect(child);
		}
	};

	collect(tree.root);

	const norm = new Map<number, number>();
	const span = Math.max(1, leaves.length - 1);

	leaves.forEach((leaf, index) => {
		norm.set(leaf.id, leaves.length === 1 ? 0.5 : index / span);
	});

	const assign = (entry: WarRoomNode): number => {
		if (entry.children.length === 0) {
			return norm.get(entry.id) ?? 0.5;
		}

		let total = 0;
		let count = 0;

		for (const id of entry.children) {
			const child = tree.nodes[id];

			if (child === undefined) continue;

			total += assign(child);
			count += 1;
		}

		const average = count === 0 ? 0.5 : total / count;
		norm.set(entry.id, average);

		return average;
	};

	assign(tree.root);

	const depthSpan = Math.max(1, tree.maxDepth);

	for (const entry of tree.nodes) {
		x.set(entry.id, PAD_LEFT + (entry.depth / depthSpan) * usableWidth);
		y.set(entry.id, PAD_TOP + (norm.get(entry.id) ?? 0.5) * usableHeight);
	}

	return { x, y };
};

const token = (name: string): string =>
	name === "" ? "—" : name.replace(/_/g, " ").toLowerCase();

/*
drawWarRoomTree paints the search. Every visual weight is a search statistic:
nothing here is styling that does not also mean something.
*/
export const drawWarRoomTree = (
	context: CanvasRenderingContext2D,
	tree: WarRoomTree,
	layout: WarRoomLayout,
	width: number,
	height: number,
	styles: {
		line: string;
		accent: string;
		text: string;
		muted: string;
		up: string;
		down: string;
	},
): void => {
	context.clearRect(0, 0, width, height);

	if (tree.root === null) {
		return;
	}

	context.lineCap = "round";
	context.lineJoin = "round";

	// Edges first, so nodes sit above them.
	for (const entry of tree.nodes) {
		if (entry.parent < 0) continue;

		const parent = tree.nodes[entry.parent];

		if (parent === undefined) continue;

		const fromX = layout.x.get(parent.id) ?? 0;
		const fromY = layout.y.get(parent.id) ?? 0;
		const toX = layout.x.get(entry.id) ?? 0;
		const toY = layout.y.get(entry.id) ?? 0;

		// Thickness is real rollout share: the branches the search actually
		// spent its budget on read as the heavy ones.
		const share = tree.peakVisits === 0 ? 0 : entry.node.visits / tree.peakVisits;
		const onPath = entry.onPath && parent.onPath;

		context.beginPath();
		context.moveTo(fromX, fromY);
		context.bezierCurveTo(
			fromX + (toX - fromX) * 0.5,
			fromY,
			fromX + (toX - fromX) * 0.5,
			toY,
			toX,
			toY,
		);

		context.strokeStyle = onPath ? styles.accent : styles.line;
		context.globalAlpha = entry.node.pruned ? 0.22 : onPath ? 0.95 : 0.34;
		context.lineWidth = onPath ? 1.9 + share * 2.2 : 0.6 + share * 2.0;
		context.stroke();
		context.globalAlpha = 1;
	}

	context.font =
		'9px ui-monospace, SFMono-Regular, Menlo, "Roboto Mono", monospace';
	context.textBaseline = "middle";

	for (const entry of tree.nodes) {
		const nodeX = layout.x.get(entry.id) ?? 0;
		const nodeY = layout.y.get(entry.id) ?? 0;
		const real = entry.node.visits;
		const virtual = entry.node.counterfactualMass;
		const total = real + virtual;

		// Radius carries total evidence; the wedge inside carries how much of
		// it was actually rolled out rather than imagined.
		const weight = tree.peakVisits === 0 ? 0 : total / (tree.peakVisits + 1);
		const radius = 2.6 + Math.min(1, weight) * 4.4;
		const realShare = total === 0 ? 0 : real / total;

		context.globalAlpha = entry.node.pruned ? 0.3 : 1;

		// The imagined portion is drawn as the full disc in accent, with the
		// rolled-out portion overlaid as a wedge. A node that is mostly accent
		// is a node the search mostly did not visit.
		if (virtual > 0) {
			context.beginPath();
			context.arc(nodeX, nodeY, radius, 0, Math.PI * 2);
			context.fillStyle = styles.accent;
			context.globalAlpha = entry.node.pruned ? 0.18 : 0.42;
			context.fill();
			context.globalAlpha = entry.node.pruned ? 0.3 : 1;
		}

		if (realShare > 0) {
			context.beginPath();
			context.moveTo(nodeX, nodeY);
			context.arc(
				nodeX,
				nodeY,
				radius,
				-Math.PI / 2,
				-Math.PI / 2 + Math.PI * 2 * realShare,
			);
			context.closePath();
			context.fillStyle = entry.onPath ? styles.accent : styles.text;
			context.fill();
		}

		context.beginPath();
		context.arc(nodeX, nodeY, radius, 0, Math.PI * 2);
		context.strokeStyle = entry.onPath ? styles.accent : styles.line;
		context.lineWidth = entry.onPath ? 1.3 : 0.7;
		context.stroke();

		// A causally rejected branch is struck through: the structural model
		// condemned it, and it must not read as merely unexplored.
		if (entry.node.pruned) {
			context.beginPath();
			context.moveTo(nodeX - radius - 2.5, nodeY);
			context.lineTo(nodeX + radius + 2.5, nodeY);
			context.strokeStyle = styles.down;
			context.lineWidth = 1;
			context.globalAlpha = 0.85;
			context.stroke();
		}

		context.globalAlpha = 1;

		if (entry.depth === 0) {
			continue;
		}

		const label = token(entry.node.action);
		const isLeaf = entry.children.length === 0;

		// Leaves label to the right; interior nodes label above, so a label
		// never lands on the edge leaving the node it belongs to.
		context.fillStyle = entry.onPath ? styles.accent : styles.muted;
		context.globalAlpha = entry.node.pruned ? 0.45 : 1;

		if (isLeaf) {
			context.textAlign = "left";
			context.fillText(label, nodeX + radius + 5, nodeY);
		} else {
			context.textAlign = "center";
			context.fillText(label, nodeX, nodeY - radius - 7);
		}

		// The reward the branch carries, on the principal variation only —
		// every node would be unreadable, and the selected path is the claim
		// the decision actually rests on.
		if (entry.onPath && entry.node.visits > 0) {
			context.fillStyle = entry.node.meanReward >= 0 ? styles.up : styles.down;
			context.textAlign = "center";
			context.fillText(
				entry.node.meanReward.toFixed(3),
				nodeX,
				nodeY + radius + 8,
			);
		}

		context.globalAlpha = 1;
	}

	context.textAlign = "left";
};
