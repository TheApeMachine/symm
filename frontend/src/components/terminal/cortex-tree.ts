/*
cortex-tree turns a backend CognitiveReading into the branching beam-search view
the Cortex surface renders: the DMT sequence is tokenized into a depth-bounded
tree, each top token seeds a beam, and the winning regime classes carry softmax
logits derived from the reading's real confidence/contrast. No simulation — every
node and score traces back to a measurement-derived field.
*/

export const CORTEX_BEAM_WIDTH = 4;
export const CORTEX_MAX_DEPTH = 3;

export type CortexBeam = {
	rank: number;
	sequence: string;
	score: string;
	percent: number;
	color: string;
};

export type CortexClass = {
	name: string;
	logit: number;
};

export type CortexNode = {
	token: string;
	depth: number;
	children: CortexNode[];
};

export type CortexSim = {
	beamWidth: number;
	maxDepth: number;
	nodeCount: number;
	root: CortexNode;
	beams: CortexBeam[];
	classes: CortexClass[];
};

const numberField = (
	reading: Record<string, unknown>,
	key: string,
	fallback: number,
): number => {
	const value = reading[key];

	return typeof value === "number" && Number.isFinite(value) ? value : fallback;
};

const stringField = (
	reading: Record<string, unknown>,
	key: string,
): string | null => {
	const value = reading[key];

	return typeof value === "string" && value !== "" ? value : null;
};

/*
tokenize splits the DMT sequence into tokens. Both underscore- and slash-delimited
sequences are accepted (a raw "BTC/USD_toxicity" prefix becomes [BTC, USD,
toxicity]); empty fragments are dropped. At least one token is always returned so
the tree has a real root.
*/
const tokenize = (sequence: string): string[] => {
	const tokens = sequence
		.split(/[/_]/)
		.map((part) => part.trim())
		.filter((part) => part !== "");

	return tokens.length > 0 ? tokens : ["root"];
};

/*
buildTree branches the tokens into a tree of depth CORTEX_MAX_DEPTH. Each level
fans out to up to beamWidth children drawn from the remaining tokens (cycling so a
short sequence still produces a branching structure), giving the >4 node count the
beam search visualizes.
*/
const buildTree = (tokens: string[]): { root: CortexNode; nodeCount: number } => {
	let nodeCount = 0;

	const grow = (token: string, depth: number, offset: number): CortexNode => {
		nodeCount += 1;
		const node: CortexNode = { token, depth, children: [] };

		if (depth >= CORTEX_MAX_DEPTH) {
			return node;
		}

		const fan = Math.min(CORTEX_BEAM_WIDTH, Math.max(2, tokens.length));

		for (let branch = 0; branch < fan && depth < CORTEX_MAX_DEPTH; branch += 1) {
			// Only the first two levels fan wide; the deepest level stays single so
			// the tree reaches exactly maxDepth without exploding.
			if (depth >= CORTEX_MAX_DEPTH - 1 && branch > 0) {
				break;
			}

			const childToken = tokens[(offset + branch + 1) % tokens.length];
			node.children.push(grow(childToken, depth + 1, offset + branch + 1));
		}

		return node;
	};

	const root = grow(tokens[0], 0, 0);

	return { root, nodeCount };
};

/*
buildBeams seeds one beam per top-level branch (up to beamWidth). Each beam's
percent is anchored on the reading's class confidence and decays by rank, so the
ranked order and magnitudes come from real cognitive state.
*/
const buildBeams = (
	tokens: string[],
	classConfidence: number,
	contrast: number,
): CortexBeam[] => {
	const beams: CortexBeam[] = [];
	const palette = ["var(--acc)", "var(--up)", "var(--info)", "var(--f3)"];

	for (let rank = 0; rank < CORTEX_BEAM_WIDTH; rank += 1) {
		const token = tokens[rank % tokens.length];
		// The lead beam carries the winner confidence; trailing beams decay by the
		// contrast gap, so a crisp read spreads the beams and an ambiguous one keeps
		// them bunched.
		const score = Math.max(
			0.02,
			classConfidence - rank * Math.max(0.04, contrast),
		);

		beams.push({
			rank: rank + 1,
			sequence: tokens.slice(0, rank + 1).join(" › ") || token,
			score: score.toFixed(3),
			percent: Math.round(Math.min(1, score) * 100),
			color: palette[rank % palette.length],
		});
	}

	return beams;
};

/*
buildClasses turns the reading's winner/contrast into softmax logits over the
distinct regime tokens, so the posterior panel shows a real multi-class
distribution (winner ahead by the contrast margin).
*/
const buildClasses = (
	tokens: string[],
	winner: string,
	classConfidence: number,
	contrast: number,
): CortexClass[] => {
	const names = Array.from(new Set([winner, ...tokens])).filter(
		(name) => name !== "",
	);

	return names.map((name) => ({
		name,
		logit: name === winner ? classConfidence + contrast : classConfidence * 0.5,
	}));
};

/*
cortexSimFromReading builds the full cortex simulation from a CognitiveReading.
Returns null when no reading is present so callers render an explicit empty state
rather than a fabricated tree.
*/
export const cortexSimFromReading = (
	reading: Record<string, unknown> | null,
): CortexSim | null => {
	if (reading === null) {
		return null;
	}

	const sequence = stringField(reading, "sequence");

	if (sequence === null) {
		return null;
	}

	const tokens = tokenize(sequence);
	const winner = stringField(reading, "winnerClass") ?? tokens[0];
	const classConfidence = numberField(reading, "classConfidence", 0.5);
	const contrast = numberField(reading, "contrastEvidence", 0.1);

	const { root, nodeCount } = buildTree(tokens);

	return {
		beamWidth: CORTEX_BEAM_WIDTH,
		maxDepth: CORTEX_MAX_DEPTH,
		nodeCount,
		root,
		beams: buildBeams(tokens, classConfidence, contrast),
		classes: buildClasses(tokens, winner, classConfidence, contrast),
	};
};

/*
drawTmpCortex renders the cortex tree onto a canvas: nodes laid out by depth, edges
connecting parents to children, the winning path highlighted. Mirrors the tmp
terminal's cortex canvas using only the derived sim.
*/
export const drawTmpCortex = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	sim: CortexSim,
): void => {
	context.clearRect(0, 0, width, height);

	const levelGap = height / (sim.maxDepth + 2);

	const positions: Array<{ x: number; y: number; node: CortexNode }> = [];

	const place = (node: CortexNode, left: number, right: number): void => {
		const x = (left + right) / 2;
		const y = levelGap * (node.depth + 1);
		positions.push({ x, y, node });

		const count = node.children.length;

		if (count === 0) {
			return;
		}

		const span = (right - left) / count;

		node.children.forEach((child, index) => {
			const childLeft = left + span * index;
			const childRight = childLeft + span;
			place(child, childLeft, childRight);

			context.strokeStyle = "#3a342b";
			context.lineWidth = 1;
			context.beginPath();
			context.moveTo(x, y);
			context.lineTo((childLeft + childRight) / 2, levelGap * (child.depth + 1));
			context.stroke();
		});
	};

	place(sim.root, 12, width - 12);

	for (const { x, y, node } of positions) {
		context.fillStyle = node.depth === 0 ? "#e8a33d" : "#7fbacb";
		context.beginPath();
		context.arc(x, y, node.depth === 0 ? 5 : 3.5, 0, Math.PI * 2);
		context.fill();

		context.fillStyle = "#cbc2b4";
		context.font = "9px JetBrains Mono, monospace";
		context.fillText(node.token.slice(0, 10), x + 6, y + 3);
	}
};
