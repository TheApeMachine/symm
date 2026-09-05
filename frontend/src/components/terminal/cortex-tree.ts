import type {
	CognitiveBranch,
	CognitiveBeam as FrameBeam,
} from "#/collections/types";
import type { Variant } from "@/components/ui/types";

export type CortexBeam = {
	rank: number;
	sequence: string;
	score: string;
	percent: number;
	variant: Variant;
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
				key: stringField(record.key),
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
		const key = stringField(record.key);

		if (score === null || sequence === "") {
			return [];
		}

		return [{ sequence, key, score }];
	});
};

const beamsFromReading = (reading: Record<string, unknown>): CortexBeam[] => {
	const beams = frameBeams(reading);

	if (beams.length === 0) {
		return [];
	}

	const maxScore = Math.max(...beams.map((beam) => beam.score));
	const displaySequence = (sequence: string): string => {
		const tokens = sequence.split("_").filter(Boolean);

		if (tokens.length === 0) {
			return sequence;
		}

		const scoped =
			tokens[0]?.startsWith("symbol-") === true ||
			tokens[0]?.startsWith("s/") === true;
		const content = scoped ? tokens.slice(1) : tokens;
		const normalized = (content.length > 0 ? content : tokens).map((token) => {
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

			return token;
		});

		return normalized.join("_");
	};

	return beams.map((beam, index) => ({
		rank: index + 1,
		sequence: displaySequence(beam.sequence),
		score: beam.score.toFixed(2),
		percent: Math.round(Math.exp(beam.score - maxScore) * 100),
		variant: index === 0 ? "warning" : "info",
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
	_reading: Record<string, unknown>,
	beam: FrameBeam | undefined,
): Set<string> => {
	// Amber tracks the MAP beam through the radix tree (mockup legend), not the
	// sealed observation bag — that bag is a long sorted spine and reads as one path.
	// Matching runs on the machine key: beam.sequence is arrow-separated display
	// text, so splitting it on "_" yielded one un-splittable blob that matched
	// no node.
	if (beam?.key) {
		return prefixesFromSequence(beam.key);
	}

	return prefixesFromSequence("");
};

const compareSiblings = (left: CortexNode, right: CortexNode): number => {
	const byToken = left.token.localeCompare(right.token);

	if (byToken !== 0) {
		return byToken;
	}

	return left.id - right.id;
};

const treeDepth = (node: CortexNode): number => {
	if (node.children.length === 0) {
		return node.depth;
	}

	return Math.max(...node.children.map(treeDepth));
};

/*
cortexTreeFromReading rebuilds the sensory prefix tree for Cortex. Sibling order
is token-stable so probability jitter cannot reshuffle leaf geometry each tick.
*/
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
		node.children.sort(compareSiblings);
	}

	const nodes = [...byID.values()].sort((left, right) => left.id - right.id);
	const beamWidth = numberField(reading, "beamWidth");
	const maxHops = numberField(reading, "maxHops");
	const nodeCount = numberField(reading, "nodeCount");

	if (beamWidth === null || maxHops === null || nodeCount === null) {
		return null;
	}

	const rawBeams = frameBeams(reading);
	// Layout depth is the rendered tree only. maxHops is sequence metadata for
	// the beam panel — using it here pinned shallow trees into the left gutter.
	const measuredDepth = Math.max(1, treeDepth(root));

	return {
		beamWidth,
		maxDepth: measuredDepth,
		nodeCount,
		root,
		nodes,
		beams: beamsFromReading(reading),
		classes: classesFromReading(reading),
		beamPrefixes: activePrefixesFrom(reading, rawBeams[0]),
	};
};
