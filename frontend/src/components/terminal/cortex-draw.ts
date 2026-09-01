import { TERMINAL_COLORS } from "#/components/terminal/canvas";
import type { CortexNode, CortexTree } from "#/components/terminal/cortex-tree";

export type CortexLayout = {
	xByID: Map<number, number>;
	yByID: Map<number, number>;
	maxDepth: number;
};

const EDGE_LABEL = "#6b6358";
const MOCKUP_LEAF_LABEL_BUDGET = 12;

/*
LABEL_HALF_HEIGHT is half a 9.5px line plus a little breathing room, so two
labels count as colliding slightly before their glyphs actually touch.
*/
const LABEL_HALF_HEIGHT = 6;

/*
CortexLeafRoster keeps leaf Y positions stable across prediction churn. New
leaves take the largest open gap; existing leaves keep their prior ordinate so
the canvas does not reshuffle every tick.
*/
export class CortexLeafRoster {
	private slots = new Map<string, number>();

	reset(): void {
		this.slots.clear();
	}

	/*
		`leaves` arrives in the tree's own depth-first order (collectLeaves in
		layoutCortexTree walks children in their sorted sibling order), so
		leaves under the same parent are already adjacent. Ranking by that
		array position — not by re-sorting prefixes alphabetically — is what
		keeps a parent's children grouped in Y and stops sibling subtrees from
		interleaving into edges that cross on screen.
	*/
	ranks(leaves: CortexNode[]): Map<string, number> {
		const current = new Set(leaves.map((leaf) => leaf.prefix));

		for (const prefix of [...this.slots.keys()]) {
			if (!current.has(prefix)) {
				this.slots.delete(prefix);
			}
		}

		if (this.slots.size === 0) {
			for (const [index, leaf] of leaves.entries()) {
				this.slots.set(
					leaf.prefix,
					leaves.length > 1 ? index / (leaves.length - 1) : 0.5,
				);
			}

			return new Map(this.slots);
		}

		for (const [index, leaf] of leaves.entries()) {
			if (this.slots.has(leaf.prefix)) {
				continue;
			}

			const before = leaves[index - 1]?.prefix;
			const after = leaves[index + 1]?.prefix;
			this.insert(leaf.prefix, before, after);
		}

		return new Map(this.slots);
	}

	/*
		A new leaf inserts between its tree-order neighbors' existing slots when
		either is already ranked, so it lands beside its actual siblings instead
		of in whatever numeric gap happens to be largest. Falls back to the
		largest open gap only when neither neighbor has a slot yet (e.g. an
		entirely new subtree arriving at once).
	*/
	private insert(
		prefix: string,
		beforePrefix: string | undefined,
		afterPrefix: string | undefined,
	): void {
		const beforeOrdinate =
			beforePrefix === undefined ? undefined : this.slots.get(beforePrefix);
		const afterOrdinate =
			afterPrefix === undefined ? undefined : this.slots.get(afterPrefix);

		if (beforeOrdinate !== undefined && afterOrdinate !== undefined) {
			this.slots.set(prefix, (beforeOrdinate + afterOrdinate) / 2);
			return;
		}

		if (beforeOrdinate !== undefined) {
			this.slots.set(prefix, Math.min(1, beforeOrdinate + 0.01));
			return;
		}

		if (afterOrdinate !== undefined) {
			this.slots.set(prefix, Math.max(0, afterOrdinate - 0.01));
			return;
		}

		const ordinates = [...this.slots.values()].sort(
			(left, right) => left - right,
		);

		if (ordinates.length === 0) {
			this.slots.set(prefix, 0.5);
			return;
		}

		let gapFrom = 0;
		let gapTo = ordinates[0] ?? 1;
		let gapSpan = gapTo - gapFrom;

		for (let index = 0; index < ordinates.length - 1; index++) {
			const from = ordinates[index] ?? 0;
			const to = ordinates[index + 1] ?? 1;
			const span = to - from;

			if (span <= gapSpan) {
				continue;
			}

			gapFrom = from;
			gapTo = to;
			gapSpan = span;
		}

		const tail = ordinates[ordinates.length - 1] ?? 0;
		const tailSpan = 1 - tail;

		if (tailSpan > gapSpan) {
			this.slots.set(prefix, (tail + 1) / 2);
			return;
		}

		this.slots.set(prefix, (gapFrom + gapTo) / 2);
	}
}

const isOnBeam = (tree: CortexTree, node: CortexNode): boolean =>
	tree.beamPrefixes.has(node.key);

/*
displayToken shortens live metric tokens to mockup-scale labels so the filled
canvas stays readable (cvd-absorption--positive → absorption).
*/
export const displayToken = (token: string): string => {
	if (token === "" || token === "\u2022") {
		return token;
	}

	if (token.startsWith("symbol-")) {
		const body = token.slice("symbol-".length);
		const segments = body.split("-").filter(Boolean);

		if (segments.length >= 2) {
			const quote = segments[segments.length - 1] ?? "";
			const base = segments.slice(0, -1).join("-");

			return `${base}/${quote}`.toUpperCase();
		}

		return body.toUpperCase();
	}

	if (token.startsWith("cat-")) {
		let category = token.slice("cat-".length);

		for (const suffix of ["-positive", "-negative", "-zero"]) {
			if (category.endsWith(suffix)) {
				category = category.slice(0, -suffix.length);
				break;
			}
		}

		return category;
	}

	const polarity = token.split("--");
	const head = polarity[0] ?? token;

	if (polarity.length > 1) {
		const segments = head.split("-").filter(Boolean);
		const tip = segments[segments.length - 1] ?? token;
		const sign = polarity[polarity.length - 1];

		if (sign === "positive") {
			return `${tip}+`;
		}

		if (sign === "negative") {
			return `${tip}-`;
		}

		if (sign === "zero") {
			return `${tip}0`;
		}
	}

	return head;
};

/*
layoutCortexTree places leaves on a sticky roster, then averages parents — the
same geometry the SYMM Terminal mockup uses, stretched across the full canvas.
*/
export const layoutCortexTree = (
	tree: CortexTree,
	width: number,
	height: number,
	roster: CortexLeafRoster = new CortexLeafRoster(),
): CortexLayout => {
	const padLeft = 74;
	const padRight = 116;
	const padTop = 40;
	const padBottom = 22;
	const usableWidth = Math.max(1, width - padLeft - padRight);
	const usableHeight = Math.max(1, height - padTop - padBottom);
	const leaves: CortexNode[] = [];

	const collectLeaves = (node: CortexNode): void => {
		if (node.children.length === 0) {
			leaves.push(node);
			return;
		}

		for (const child of node.children) {
			collectLeaves(child);
		}
	};

	collectLeaves(tree.root);

	const leafRanks = roster.ranks(leaves);
	const yNorm = new Map<number, number>();

	for (const leaf of leaves) {
		yNorm.set(leaf.id, leafRanks.get(leaf.prefix) ?? 0.5);
	}

	const assignY = (node: CortexNode): number => {
		if (node.children.length === 0) {
			const leaf = yNorm.get(node.id) ?? 0.5;
			yNorm.set(node.id, leaf);
			return leaf;
		}

		const childY = node.children.map(assignY);
		const average =
			childY.reduce((sum, value) => sum + value, 0) / childY.length;
		yNorm.set(node.id, average);
		return average;
	};

	assignY(tree.root);

	// Stretch to the deepest rendered node so a shallow tree still fills width.
	const maxDepth = Math.max(1, tree.maxDepth);
	const xByID = new Map<number, number>();
	const yByID = new Map<number, number>();

	for (const node of tree.nodes) {
		xByID.set(node.id, padLeft + (node.depth / maxDepth) * usableWidth);
		yByID.set(node.id, padTop + (yNorm.get(node.id) ?? 0.5) * usableHeight);
	}

	return { xByID, yByID, maxDepth };
};

/*
	Interior nodes are the categories a sequence actually passed through, so
	they stay labeled at every depth regardless of leaf density — hiding them
	past depth 2 was what made deeper hops read as unlabeled blank circles.
	Only leaf labels thin out under crowding, since a dense leaf fringe is
	where text genuinely overlaps; the cutoff softens into a lower budget
	rather than a hard on/off cliff so a moderately busy tree still shows
	some leaf text instead of none.
*/
const shouldLabel = (
	node: CortexNode,
	onBeam: boolean,
	leafCount: number,
): boolean => {
	if (node.depth === 0) {
		return false;
	}

	if (node.token.startsWith("symbol-")) {
		return false;
	}

	if (onBeam || node.children.length > 0) {
		return true;
	}

	if (node.depth <= 2) {
		return true;
	}

	return leafCount <= MOCKUP_LEAF_LABEL_BUDGET * 2;
};

const beamPathNodes = (tree: CortexTree): CortexNode[] => {
	let beamSequence = "";

	for (const prefix of tree.beamPrefixes) {
		if (prefix.length > beamSequence.length) {
			beamSequence = prefix;
		}
	}

	if (beamSequence === "") {
		return [tree.root];
	}

	const nodeByPrefix = new Map(
		tree.nodes.map((node) => [node.key, node] as const),
	);
	const tokens = beamSequence.split("_").filter(Boolean);
	const path: CortexNode[] = [tree.root];
	let prefix = "";

	for (const token of tokens) {
		prefix = prefix === "" ? token : `${prefix}_${token}`;
		const match = nodeByPrefix.get(prefix);

		if (match !== undefined) {
			path.push(match);
		}
	}

	return path;
};

/*
drawCortexTree paints the mockup radix tree: full-bleed layout, bezier branches,
amber MAP beam, edge probabilities, and a traveling pulse.
*/
export const drawCortexTree = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	tree: CortexTree,
	roster: CortexLeafRoster = new CortexLeafRoster(),
): void => {
	context.clearRect(0, 0, width, height);
	context.fillStyle = TERMINAL_COLORS.background;
	context.fillRect(0, 0, width, height);

	const layout = layoutCortexTree(tree, width, height, roster);
	const leafCount = tree.nodes.filter(
		(node) => node.children.length === 0,
	).length;

	const drawEdges = (node: CortexNode): void => {
		const fromX = layout.xByID.get(node.id);
		const fromY = layout.yByID.get(node.id);

		if (fromX === undefined || fromY === undefined) {
			return;
		}

		for (const child of node.children) {
			const toX = layout.xByID.get(child.id);
			const toY = layout.yByID.get(child.id);

			if (toX === undefined || toY === undefined) {
				continue;
			}

			const onBeam = isOnBeam(tree, node) && isOnBeam(tree, child);
			const middleX = (fromX + toX) / 2;

			context.strokeStyle = onBeam
				? TERMINAL_COLORS.amber
				: TERMINAL_COLORS.line;
			context.lineWidth = onBeam ? 2.2 : 0.6 + child.probability * 2.4;
			context.globalAlpha = onBeam ? 1 : 0.45 + child.probability * 0.4;
			context.beginPath();
			context.moveTo(fromX, fromY);
			context.bezierCurveTo(middleX, fromY, middleX, toY, toX, toY);
			context.stroke();

			if (!onBeam) {
				context.globalAlpha = 0.7;
				context.fillStyle = EDGE_LABEL;
				context.font = "8px JetBrains Mono, monospace";
				context.textAlign = "center";
				context.fillText(
					child.probability.toFixed(2),
					middleX,
					(fromY + toY) / 2 - 3,
				);
			}

			drawEdges(child);
		}
	};

	context.globalAlpha = 1;
	drawEdges(tree.root);
	context.globalAlpha = 1;

	/*
	Dots first, then labels. A label is only worth drawing if it can be read,
	so each candidate is measured against the boxes already placed and dropped
	when it would collide. Beam nodes are placed first so the path the reader
	is actually following always wins the space over an incidental neighbour.
	*/
	type LabelBox = { top: number; bottom: number; left: number; right: number };
	const placed: LabelBox[] = [];
	const pending: { node: CortexNode; x: number; y: number; onBeam: boolean }[] =
		[];

	for (const node of tree.nodes) {
		const x = layout.xByID.get(node.id);
		const y = layout.yByID.get(node.id);

		if (x === undefined || y === undefined) {
			continue;
		}

		const onBeam = isOnBeam(tree, node);
		const radius = node.depth === 0 ? 5.5 : onBeam ? 4.2 : 3;

		context.fillStyle = onBeam ? TERMINAL_COLORS.amber : "#1f1a14";
		context.strokeStyle = onBeam
			? TERMINAL_COLORS.amber
			: TERMINAL_COLORS.lineStrong;
		context.lineWidth = 1;
		context.beginPath();
		context.arc(x, y, radius, 0, Math.PI * 2);
		context.fill();
		context.stroke();

		if (!shouldLabel(node, onBeam, leafCount)) {
			continue;
		}

		pending.push({ node, x, y, onBeam });
	}

	pending.sort((left, right) => Number(right.onBeam) - Number(left.onBeam));

	for (const { node, x, y, onBeam } of pending) {
		const label = displayToken(node.token);

		context.font = `${onBeam ? "600 " : ""}9.5px JetBrains Mono, monospace`;

		const left = x + 7;
		const box: LabelBox = {
			top: y - LABEL_HALF_HEIGHT,
			bottom: y + LABEL_HALF_HEIGHT,
			left,
			right: left + context.measureText(label).width,
		};

		const collides = placed.some(
			(other) =>
				box.left < other.right &&
				box.right > other.left &&
				box.top < other.bottom &&
				box.bottom > other.top,
		);

		if (collides) {
			continue;
		}

		placed.push(box);

		context.fillStyle = onBeam
			? TERMINAL_COLORS.foreground
			: TERMINAL_COLORS.muted;
		context.textAlign = "left";
		context.fillText(label, left, y + 3.2);
	}

	const pathNodes = beamPathNodes(tree);

	if (pathNodes.length <= 1) {
		return;
	}

	const tick = (performance.now() / 1500) % 1;
	const segment = tick * (pathNodes.length - 1);
	const startIndex = Math.floor(segment);
	const fraction = segment - startIndex;
	const startID = pathNodes[startIndex]?.id;
	const endID = pathNodes[Math.min(pathNodes.length - 1, startIndex + 1)]?.id;
	const startX = startID === undefined ? undefined : layout.xByID.get(startID);
	const startY = startID === undefined ? undefined : layout.yByID.get(startID);
	const endX = endID === undefined ? undefined : layout.xByID.get(endID);
	const endY = endID === undefined ? undefined : layout.yByID.get(endID);

	if (
		startX === undefined ||
		startY === undefined ||
		endX === undefined ||
		endY === undefined
	) {
		return;
	}

	const pulseX = startX + (endX - startX) * fraction;
	const pulseY = startY + (endY - startY) * fraction;

	context.fillStyle = TERMINAL_COLORS.amber;
	context.globalAlpha = 0.95;
	context.beginPath();
	context.arc(pulseX, pulseY, 3.6, 0, Math.PI * 2);
	context.fill();
	context.globalAlpha = 0.22;
	context.beginPath();
	context.arc(pulseX, pulseY, 9, 0, Math.PI * 2);
	context.fill();
	context.globalAlpha = 1;
};
