import { TERMINAL_COLORS } from "#/components/terminal/canvas";
import { clamp, rnd } from "#/components/terminal/tmp-sim";

export type CortexNode = {
	id: number;
	token: string;
	probability: number;
	depth: number;
	children: CortexNode[];
	y?: number;
};

export type CortexClass = {
	name: string;
	logit: number;
};

export type CortexBeam = {
	rank: number;
	sequence: string;
	score: string;
	percent: number;
	color: string;
};

export type CortexSim = {
	root: CortexNode;
	classes: CortexClass[];
	beams: CortexBeam[];
	beamPath: CortexNode[];
	beamSet: Set<number>;
	beamWidth: number;
	maxDepth: number;
	nodeCount: number;
	rem: {
		phase: string;
		decay: number;
		replays: number;
		inhibition: number;
		ticks: number;
	};
};

const TOKENS = [
	"coil",
	"lift",
	"sweep",
	"ice",
	"spoof",
	"void",
	"dump",
	"hold",
	"absorb",
	"frenzy",
	"thin",
	"flush",
];

const shuffle = <T>(values: T[]): T[] => {
	const copy = values.slice();

	for (let index = copy.length - 1; index > 0; index -= 1) {
		const swap = (Math.random() * (index + 1)) | 0;
		const current = copy[index];
		copy[index] = copy[swap] ?? current;
		copy[swap] = current;
	}

	return copy;
};

const makeNode = (
	idRef: { value: number },
	token: string,
	depth: number,
	probability: number,
): CortexNode => ({
	id: idRef.value++,
	token,
	probability,
	depth,
	children: [],
});

const growTree = (
	node: CortexNode,
	depth: number,
	branch: number[],
	idRef: { value: number },
): void => {
	if (depth >= branch.length) {
		return;
	}

	const width = branch[depth] ?? 0;
	const tokens = shuffle(TOKENS).slice(0, width);
	const raw = tokens.map(() => rnd(0.25, 1));
	const sum = raw.reduce((total, value) => total + value, 0) || 1;

	node.children = tokens.map((token, index) => {
		const child = makeNode(idRef, token, depth + 1, (raw[index] ?? 0) / sum);
		growTree(child, depth + 1, branch, idRef);

		return child;
	});
};

export const computeBeams = (sim: CortexSim): void => {
	const paths: Array<{ nodes: CortexNode[]; score: number }> = [];

	const expand = (node: CortexNode, acc: CortexNode[], score: number) => {
		if (node.children.length === 0) {
			paths.push({ nodes: acc, score });
			return;
		}

		for (const child of node.children) {
			expand(
				child,
				acc.concat(child),
				score + Math.log(Math.max(child.probability, 1e-6)),
			);
		}
	};

	expand(sim.root, [sim.root], 0);
	paths.sort((left, right) => right.score - left.score);
	const top = paths.slice(0, sim.beamWidth);
	const beamPath = top[0]?.nodes ?? [];
	const maxScore = top[0]?.score ?? 0;

	sim.beams = top.map((path, index) => ({
		rank: index + 1,
		sequence: path.nodes
			.slice(1)
			.map((node) => node.token)
			.join("_"),
		score: path.score.toFixed(2),
		percent: Math.round(Math.exp(path.score - maxScore) * 100),
		color: index === 0 ? "var(--acc)" : "var(--info)",
	}));
	sim.beamPath = beamPath;
	sim.beamSet = new Set(beamPath.map((node) => node.id));
};

export const createCortexSim = (): CortexSim => {
	const idRef = { value: 0 };
	const root = makeNode(idRef, "•", 0, 1);
	const branch = [3, 2, 2];
	growTree(root, 0, branch, idRef);

	let nodeCount = 0;
	const count = (node: CortexNode) => {
		nodeCount += 1;
		node.children.forEach(count);
	};
	count(root);

	const sim: CortexSim = {
		root,
		classes: ["trend", "drive", "leadlag", "scarcity", "pump"].map((name) => ({
			name,
			logit: rnd(-0.6, 0.6),
		})),
		beams: [],
		beamPath: [],
		beamSet: new Set(),
		beamWidth: 4,
		maxDepth: branch.length,
		nodeCount,
		rem: {
			phase: "awake",
			decay: 0.92,
			replays: 0,
			inhibition: 0.1,
			ticks: 0,
		},
	};

	computeBeams(sim);

	return sim;
};

export const tickCortexSim = (sim: CortexSim): void => {
	const renorm = (node: CortexNode) => {
		if (node.children.length === 0) {
			return;
		}

		let sum = 0;

		for (const child of node.children) {
			child.probability = clamp(child.probability + rnd(-0.06, 0.06), 0.04, 1);
			sum += child.probability;
		}

		for (const child of node.children) {
			child.probability /= sum;
			renorm(child);
		}
	};

	renorm(sim.root);

	if (Math.random() < 0.12) {
		const pickLeaf = (node: CortexNode): CortexNode => {
			if (node.children.length === 0) {
				return node;
			}

			return pickLeaf(
				node.children[(Math.random() * node.children.length) | 0] ?? node,
			);
		};

		pickLeaf(sim.root).token =
			TOKENS[(Math.random() * TOKENS.length) | 0] ?? "hold";
	}

	computeBeams(sim);

	for (const cortexClass of sim.classes) {
		cortexClass.logit = clamp(cortexClass.logit + rnd(-0.12, 0.12), -1.4, 1.6);
	}

	const rem = sim.rem;
	rem.ticks += 1;
	const cycle = rem.ticks % 23;
	rem.phase = cycle < 14 ? "awake" : cycle < 19 ? "rem-replay" : "consolidate";

	if (rem.phase === "rem-replay") {
		rem.replays += rnd(1, 4) | 0;
		rem.inhibition = clamp(rem.inhibition + 0.04, 0.1, 0.95);
		return;
	}

	if (rem.phase === "consolidate") {
		rem.decay = clamp(rem.replays / (rem.replays + sim.nodeCount), 0.3, 0.99);
		return;
	}

	rem.inhibition = clamp(rem.inhibition - 0.05, 0.08, 0.95);
};

export const drawTmpCortex = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	sim: CortexSim,
): void => {
	context.clearRect(0, 0, width, height);

	const padLeft = 74;
	const padRight = 116;
	const padTop = 40;
	const padBottom = 22;
	const maxDepth = sim.maxDepth;
	const leaves: CortexNode[] = [];
	const collect = (node: CortexNode) => {
		if (node.children.length === 0) {
			leaves.push(node);
			return;
		}

		node.children.forEach(collect);
	};
	collect(sim.root);

	leaves.forEach((node, index) => {
		node.y = leaves.length > 1 ? index / (leaves.length - 1) : 0.5;
	});

	const setY = (node: CortexNode): number => {
		if (node.children.length === 0) {
			return node.y ?? 0.5;
		}

		const values = node.children.map(setY);
		node.y = values.reduce((sum, value) => sum + value, 0) / values.length;

		return node.y;
	};
	setY(sim.root);

	const xAt = (depth: number) =>
		padLeft + (depth / maxDepth) * (width - padLeft - padRight);
	const yAt = (value: number) => padTop + value * (height - padTop - padBottom);
	const accent = TERMINAL_COLORS.amber;

	const drawEdges = (node: CortexNode) => {
		for (const child of node.children) {
			const onBeam = sim.beamSet.has(node.id) && sim.beamSet.has(child.id);
			const x1 = xAt(node.depth);
			const y1 = yAt(node.y ?? 0.5);
			const x2 = xAt(child.depth);
			const y2 = yAt(child.y ?? 0.5);
			const midX = (x1 + x2) / 2;
			context.strokeStyle = onBeam ? accent : "#2b251e";
			context.lineWidth = onBeam ? 2.2 : 0.6 + child.probability * 2.4;
			context.globalAlpha = onBeam ? 1 : 0.45 + child.probability * 0.4;
			context.beginPath();
			context.moveTo(x1, y1);
			context.bezierCurveTo(midX, y1, midX, y2, x2, y2);
			context.stroke();

			if (!onBeam) {
				context.globalAlpha = 0.7;
				context.fillStyle = "#6b6358";
				context.font = "8px JetBrains Mono, monospace";
				context.textAlign = "center";
				context.fillText(child.probability.toFixed(2), midX, (y1 + y2) / 2 - 3);
			}

			drawEdges(child);
		}
	};

	context.globalAlpha = 1;
	drawEdges(sim.root);
	context.globalAlpha = 1;

	const drawNode = (node: CortexNode) => {
		const x = xAt(node.depth);
		const y = yAt(node.y ?? 0.5);
		const onBeam = sim.beamSet.has(node.id);
		const radius = node.depth === 0 ? 5.5 : onBeam ? 4.2 : 3;
		context.fillStyle = onBeam ? accent : "#1f1a14";
		context.strokeStyle = onBeam ? accent : "#3a342b";
		context.lineWidth = 1;
		context.beginPath();
		context.arc(x, y, radius, 0, Math.PI * 2);
		context.fill();
		context.stroke();

		if (node.depth > 0 && (onBeam || node.children.length === 0)) {
			context.fillStyle = onBeam ? TERMINAL_COLORS.foreground : "#938a7e";
			context.font = `${onBeam ? "600 " : ""}9.5px JetBrains Mono, monospace`;
			context.textAlign = "left";
			context.fillText(node.token, x + 7, y + 3.2);
		}

		node.children.forEach(drawNode);
	};

	drawNode(sim.root);

	const path = sim.beamPath;

	if (path.length > 1) {
		const travel = (performance.now() / 1500) % 1;
		const segment = travel * (path.length - 1);
		const index = Math.floor(segment);
		const fraction = segment - index;
		const start = path[index] ?? path[0];
		const end = path[Math.min(path.length - 1, index + 1)] ?? start;
		const px =
			xAt(start.depth) + (xAt(end.depth) - xAt(start.depth)) * fraction;
		const py =
			yAt(start.y ?? 0.5) +
			(yAt(end.y ?? 0.5) - yAt(start.y ?? 0.5)) * fraction;
		context.fillStyle = accent;
		context.globalAlpha = 0.95;
		context.beginPath();
		context.arc(px, py, 3.6, 0, Math.PI * 2);
		context.fill();
		context.globalAlpha = 0.22;
		context.beginPath();
		context.arc(px, py, 9, 0, Math.PI * 2);
		context.fill();
		context.globalAlpha = 1;
	}
};

export const cortexPosterior = (sim: CortexSim) => {
	const logits = sim.classes.map((entry) => entry.logit);
	const maxLogit = Math.max(...logits);
	const exponents = logits.map((logit) => Math.exp((logit - maxLogit) * 2));
	const normalizer = exponents.reduce((sum, value) => sum + value, 0) || 1;
	const posterior = sim.classes.map((entry, index) => ({
		name: entry.name,
		probability: (exponents[index] ?? 0) / normalizer,
	}));
	const sorted = posterior
		.slice()
		.sort((left, right) => right.probability - left.probability);
	const winner = sorted[0] ?? { name: "", probability: 0 };
	const runner = sorted[1] ?? { name: "", probability: 1e-6 };
	const winnerBits = -Math.log2(Math.max(winner.probability, 1e-9));
	const runnerBits = -Math.log2(Math.max(runner.probability, 1e-9));
	const kl =
		winner.probability *
		Math.log2(winner.probability / Math.max(runner.probability, 1e-9));
	const rootChildren = sim.root.children;
	const entropy = -rootChildren.reduce(
		(sum, child) =>
			sum +
			(child.probability > 0
				? child.probability * Math.log2(child.probability)
				: 0),
		0,
	);
	const entropyMax = Math.log2(Math.max(rootChildren.length, 2));
	const threshold = entropyMax * 0.82;

	return {
		classes: posterior.map((entry) => ({
			name: entry.name,
			percent: Math.round(entry.probability * 100),
			percentText: `${Math.round(entry.probability * 100)}%`,
			color: entry.name === winner.name ? "var(--acc)" : "var(--info)",
			foreground: entry.name === winner.name ? "var(--f1)" : "var(--f3)",
		})),
		winner: winner.name,
		winnerPercent: `${Math.round(winner.probability * 100)}%`,
		winnerBits: winnerBits.toFixed(2),
		runnerBits: runnerBits.toFixed(2),
		kl: kl.toFixed(3),
		marginPercent: Math.round(clamp((runnerBits - winnerBits) / 4, 0, 1) * 100),
		entropy: entropy.toFixed(2),
		entropyThreshold: threshold.toFixed(2),
		entropyPercent: Math.round(clamp(entropy / entropyMax, 0, 1) * 100),
		ambiguous: entropy >= threshold,
	};
};

export const cortexSimFromReading = (
	reading: {
		sequence: string;
		winnerClass: string;
		regimePrefix: string;
		classConfidence: number;
		lookaheadPaths: number;
		contrastEvidence: number;
		entropyBits: number;
		entropyThreshold: number;
		sideline: boolean;
		ambiguous: boolean;
	} | null,
): CortexSim | null => {
	if (reading === null || reading.sequence.trim() === "") {
		return null;
	}

	const tokens = reading.sequence
		.split(/[-_]+/)
		.filter((token) => token !== "");

	if (tokens.length === 0) {
		return null;
	}

	const idRef = { value: 0 };
	const root = makeNode(idRef, "s", 0, 1);
	const branch = [3, 2, 2];
	growTree(root, 0, branch, idRef);

	let cursor = root;

	for (const token of tokens) {
		if (cursor.children.length === 0) {
			break;
		}

		cursor.children[0].token = token;
		cursor = cursor.children[0];
	}

	let nodeCount = 0;
	const count = (node: CortexNode) => {
		nodeCount += 1;
		node.children.forEach(count);
	};
	count(root);

	const sim: CortexSim = {
		root,
		classes: [
			{ name: reading.winnerClass || "trend", logit: reading.classConfidence },
			{
				name: reading.regimePrefix || "drive",
				logit: 1 - reading.classConfidence,
			},
			{ name: "leadlag", logit: reading.contrastEvidence },
			{
				name: "scarcity",
				logit: reading.entropyBits / Math.max(reading.entropyThreshold, 1),
			},
			{
				name: "pump",
				logit: reading.lookaheadPaths / Math.max(tokens.length, 1),
			},
		],
		beams: [],
		beamPath: [],
		beamSet: new Set(),
		beamWidth: 4,
		maxDepth: branch.length,
		nodeCount,
		rem: {
			phase: reading.sideline
				? "sideline"
				: reading.ambiguous
					? "rem-replay"
					: "awake",
			decay: clamp(reading.contrastEvidence, 0, 0.99),
			replays: reading.lookaheadPaths,
			inhibition: clamp(
				reading.entropyBits / Math.max(reading.entropyThreshold, 1),
				0,
				0.95,
			),
			ticks: 0,
		},
	};

	computeBeams(sim);

	return sim;
};
