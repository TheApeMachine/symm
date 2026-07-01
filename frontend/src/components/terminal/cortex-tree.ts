import type {
	CognitiveBranch,
	CognitiveBeam as FrameBeam,
} from "#/collections/cognitive";
import { TERMINAL_COLORS } from "#/components/terminal/canvas";

export type CortexBeam = {
	rank: number;
	sequence: string;
	score: string;
	percent: number;
	color: string;
};

export type CortexClass = {
	name: string;
	probability: number;
};

export type CortexNode = CognitiveBranch & {
	children: CortexNode[];
};

export type CortexTree = {
	beamWidth: number;
	maxDepth: number;
	nodeCount: number;
	root: CortexNode;
	nodes: CortexNode[];
	beams: CortexBeam[];
	classes: CortexClass[];
	beamPrefixes: Set<string>;
};

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const numberField = (
	reading: Record<string, unknown>,
	key: string,
): number | null => finite(reading[key]);

const stringField = (value: unknown, fallback = ""): string =>
	typeof value === "string" ? value : fallback;

const branchesFromReading = (
	reading: Record<string, unknown>,
): CognitiveBranch[] => {
	const branches = reading.branches;

	if (!Array.isArray(branches)) {
		return [];
	}

	return branches.flatMap((entry) => {
		if (entry === null || typeof entry !== "object") {
			return [];
		}

		const record = entry as Record<string, unknown>;
		const id = finite(record.id);
		const parentId = finite(record.parentId);
		const depth = finite(record.depth);
		const probability = finite(record.probability);
		const count = finite(record.count);

		if (
			id === null ||
			parentId === null ||
			depth === null ||
			probability === null ||
			count === null
		) {
			return [];
		}

		return [
			{
				id,
				parentId,
				token: stringField(record.token, "node"),
				prefix: stringField(record.prefix),
				depth,
				probability,
				count,
			},
		];
	});
};

const frameBeams = (reading: Record<string, unknown>): FrameBeam[] => {
	const beams = reading.beams;

	if (!Array.isArray(beams)) {
		return [];
	}

	return beams.flatMap((entry) => {
		if (entry === null || typeof entry !== "object") {
			return [];
		}

		const record = entry as Record<string, unknown>;
		const score = finite(record.score);
		const sequence = stringField(record.sequence);

		if (score === null || sequence === "") {
			return [];
		}

		return [{ sequence, score }];
	});
};

const beamsFromReading = (reading: Record<string, unknown>): CortexBeam[] => {
	const beams = frameBeams(reading);

	if (beams.length === 0) {
		return [];
	}

	const maxScore = Math.max(...beams.map((beam) => beam.score));

	return beams.map((beam, index) => ({
		rank: index + 1,
		sequence: beam.sequence,
		score: beam.score.toFixed(2),
		percent: Math.round(Math.exp(beam.score - maxScore) * 100),
		color: index === 0 ? "var(--acc)" : "var(--info)",
	}));
};

const classesFromReading = (
	reading: Record<string, unknown>,
): CortexClass[] => {
	const classes = reading.classes;

	if (!Array.isArray(classes)) {
		return [];
	}

	return classes
		.flatMap((entry) => {
			if (entry === null || typeof entry !== "object") {
				return [];
			}

			const record = entry as Record<string, unknown>;
			const probability = finite(record.probability);
			const name = stringField(record.name);

			if (probability === null || name === "") {
				return [];
			}

			return [{ name, probability }];
		})
		.sort((left, right) => right.probability - left.probability);
};

const prefixesFromSequence = (sequence: string): Set<string> => {
	const prefixes = new Set<string>([""]);

	if (sequence === "") {
		return prefixes;
	}

	const tokens = sequence.split("_").filter(Boolean);
	let prefix = "";

	for (const token of tokens) {
		prefix = prefix === "" ? token : `${prefix}_${token}`;
		prefixes.add(prefix);
	}

	return prefixes;
};

const activePrefixesFrom = (
	reading: Record<string, unknown>,
	beam: FrameBeam | undefined,
): Set<string> => {
	const sequence = stringField(reading.sequence);

	if (sequence !== "") {
		return prefixesFromSequence(sequence);
	}

	return prefixesFromSequence(beam?.sequence ?? "");
};

export const cortexTreeFromReading = (
	reading: Record<string, unknown> | null,
): CortexTree | null => {
	if (reading === null) {
		return null;
	}

	const branches = branchesFromReading(reading);

	if (branches.length === 0) {
		return null;
	}

	const byID = new Map<number, CortexNode>();

	for (const branch of branches) {
		byID.set(branch.id, { ...branch, children: [] });
	}

	let root: CortexNode | null = null;

	for (const node of byID.values()) {
		if (node.parentId < 0 || node.depth === 0) {
			root = node;
			continue;
		}

		byID.get(node.parentId)?.children.push(node);
	}

	if (root === null) {
		return null;
	}

	for (const node of byID.values()) {
		node.children.sort((left, right) => {
			if (left.probability === right.probability) {
				return left.token.localeCompare(right.token);
			}

			return right.probability - left.probability;
		});
	}

	const nodes = [...byID.values()].sort((left, right) => left.id - right.id);
	const beamWidth = numberField(reading, "beamWidth");
	const maxHops = numberField(reading, "maxHops");
	const nodeCount = numberField(reading, "nodeCount");

	if (beamWidth === null || maxHops === null || nodeCount === null) {
		return null;
	}

	const maxDepth = Math.max(1, maxHops);
	const rawBeams = frameBeams(reading);

	return {
		beamWidth,
		maxDepth,
		nodeCount,
		root,
		nodes,
		beams: beamsFromReading(reading),
		classes: classesFromReading(reading),
		beamPrefixes: activePrefixesFrom(reading, rawBeams[0]),
	};
};

type PositionedNode = {
	node: CortexNode;
	x: number;
	y: number;
};

const isOnBeam = (tree: CortexTree, node: CortexNode): boolean =>
	tree.beamPrefixes.has(node.prefix);

const truncate = (value: string, maxLength: number): string =>
	value.length <= maxLength ? value : value.slice(0, maxLength - 1);

export const drawCortexTree = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	tree: CortexTree,
): void => {
	context.clearRect(0, 0, width, height);
	context.fillStyle = TERMINAL_COLORS.background;
	context.fillRect(0, 0, width, height);

	const padLeft = 74;
	const padRight = 116;
	const padTop = 44;
	const padBottom = 26;
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

	const leafY = new Map<number, number>();

	for (const [index, leaf] of leaves.entries()) {
		leafY.set(leaf.id, leaves.length > 1 ? index / (leaves.length - 1) : 0.5);
	}

	const assignY = (node: CortexNode): number => {
		if (node.children.length === 0) {
			return leafY.get(node.id) ?? 0.5;
		}

		const childY = node.children.map(assignY);

		return childY.reduce((sum, value) => sum + value, 0) / childY.length;
	};

	const yByID = new Map<number, number>();

	const recordY = (node: CortexNode): number => {
		const y = assignY(node);
		yByID.set(node.id, y);

		for (const child of node.children) {
			recordY(child);
		}

		return y;
	};

	recordY(tree.root);

	const xFor = (depth: number): number =>
		padLeft + (depth / Math.max(1, tree.maxDepth)) * usableWidth;
	const yFor = (node: CortexNode): number =>
		padTop + (yByID.get(node.id) ?? 0.5) * usableHeight;
	const positions = new Map<number, PositionedNode>();

	for (const node of tree.nodes) {
		positions.set(node.id, {
			node,
			x: xFor(node.depth),
			y: yFor(node),
		});
	}

	const drawEdges = (node: CortexNode): void => {
		const from = positions.get(node.id);

		if (from === undefined) {
			return;
		}

		for (const child of node.children) {
			const to = positions.get(child.id);

			if (to === undefined) {
				continue;
			}

			const onBeam = isOnBeam(tree, node) && isOnBeam(tree, child);
			const middleX = (from.x + to.x) / 2;

			context.strokeStyle = onBeam
				? TERMINAL_COLORS.amber
				: TERMINAL_COLORS.line;
			context.lineWidth = onBeam ? 2.2 : 0.6 + child.probability * 2.4;
			context.globalAlpha = onBeam ? 1 : 0.45 + child.probability * 0.4;
			context.beginPath();
			context.moveTo(from.x, from.y);
			context.bezierCurveTo(middleX, from.y, middleX, to.y, to.x, to.y);
			context.stroke();

			if (!onBeam) {
				context.globalAlpha = 0.7;
				context.fillStyle = TERMINAL_COLORS.muted;
				context.font = "8px JetBrains Mono, monospace";
				context.textAlign = "center";
				context.fillText(
					child.probability.toFixed(2),
					middleX,
					(from.y + to.y) / 2 - 3,
				);
			}

			drawEdges(child);
		}
	};

	drawEdges(tree.root);
	context.globalAlpha = 1;

	for (const positioned of positions.values()) {
		const { node, x, y } = positioned;
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

		if (node.depth > 0 && (onBeam || node.children.length === 0)) {
			context.fillStyle = onBeam
				? TERMINAL_COLORS.foreground
				: TERMINAL_COLORS.muted;
			context.font = `${onBeam ? "600 " : ""}9.5px JetBrains Mono, monospace`;
			context.textAlign = "left";
			context.fillText(truncate(node.token, 12), x + 7, y + 3.2);
		}
	}
};
