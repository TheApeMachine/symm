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
CortexLeafRoster keeps leaf Y positions stable across prediction churn. New
leaves take the largest open gap; existing leaves keep their prior ordinate so
the canvas does not reshuffle every tick.
*/
export class CortexLeafRoster {
	private slots = new Map<string, number>();

	reset(): void {
		this.slots.clear();
	}

	ranks(leaves: CortexNode[]): Map<string, number> {
		const current = new Set(leaves.map((leaf) => leaf.prefix));

		for (const prefix of [...this.slots.keys()]) {
			if (!current.has(prefix)) {
				this.slots.delete(prefix);
			}
		}

		const arriving = [...leaves]
			.filter((leaf) => !this.slots.has(leaf.prefix))
			.sort((left, right) => left.prefix.localeCompare(right.prefix));

		if (this.slots.size === 0) {
			const initial = [...leaves].sort((left, right) =>
				left.prefix.localeCompare(right.prefix),
			);

			for (const [index, leaf] of initial.entries()) {
				this.slots.set(
					leaf.prefix,
					initial.length > 1 ? index / (initial.length - 1) : 0.5,
				);
			}

			return new Map(this.slots);
		}

		for (const leaf of arriving) {
			this.insert(leaf.prefix);
		}

		return new Map(this.slots);
	}

	private insert(prefix: string): void {
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
	tree.beamPrefixes.has(node.prefix);

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

	if (node.depth <= 2) {
		return true;
	}

	if (onBeam) {
		return true;
	}

	if (node.children.length > 0) {
		return false;
	}

	return leafCount <= MOCKUP_LEAF_LABEL_BUDGET;
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
		tree.nodes.map((node) => [node.prefix, node] as const),
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

		context.fillStyle = onBeam
			? TERMINAL_COLORS.foreground
			: TERMINAL_COLORS.muted;
		context.font = `${onBeam ? "600 " : ""}9.5px JetBrains Mono, monospace`;
		context.textAlign = "left";
		context.fillText(displayToken(node.token), x + 7, y + 3.2);
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
