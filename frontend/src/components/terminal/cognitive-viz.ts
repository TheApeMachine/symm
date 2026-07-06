import {
	type CortexBeam,
	type CortexTree,
	cortexTreeFromReading,
	drawCortexTree,
} from "#/components/terminal/cortex-tree";

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

export type CognitivePosterior = {
	winner: string;
	winnerPercent: string;
	winnerBits: string;
	runnerBits: string;
	kl: string;
	marginPercent: number;
	entropy: string;
	entropyThreshold: string;
	entropyPercent: number;
	ambiguous: boolean;
	classes: Array<{
		name: string;
		percent: number;
		color: string;
		foreground: string;
	}>;
};

export const cognitivePosteriorFromReading = (
	reading: Record<string, unknown> | null,
): CognitivePosterior => {
	const tree = cortexTreeFromReading(reading);

	if (tree === null) {
		return {
			winner: "waiting",
			winnerPercent: "0%",
			winnerBits: "0.00",
			runnerBits: "0.00",
			kl: "0.000",
			marginPercent: 0,
			entropy: "0.00",
			entropyThreshold: "0.00",
			entropyPercent: 0,
			ambiguous: false,
			classes: [],
		};
	}

	const sorted = tree.classes
		.map((entry) => ({
			name: entry.name,
			probability: entry.probability,
		}))
		.sort((left, right) => right.probability - left.probability);
	const winner = sorted[0];
	const runner = sorted[1];

	if (winner === undefined) {
		return {
			winner: "waiting",
			winnerPercent: "0%",
			winnerBits: "0.00",
			runnerBits: "—",
			kl: "—",
			marginPercent: 0,
			entropy:
				typeof reading?.entropyBits === "number"
					? reading.entropyBits.toFixed(2)
					: "0.00",
			entropyThreshold:
				typeof reading?.entropyThreshold === "number"
					? reading.entropyThreshold.toFixed(2)
					: "0.00",
			entropyPercent: 0,
			ambiguous: reading?.ambiguous === true,
			classes: [],
		};
	}

	const winnerProbability = clamp(winner.probability, 0.001, 0.999);
	const runnerProbability =
		runner === undefined ? null : clamp(runner.probability, 0.001, 0.999);
	const winnerBits = -Math.log2(winnerProbability);
	const runnerBits =
		runnerProbability === null ? null : -Math.log2(runnerProbability);
	const kl =
		runnerProbability === null
			? null
			: winnerProbability *
				Math.log2(winnerProbability / Math.max(runnerProbability, 1e-9));
	const entropyRatio =
		reading !== null &&
		typeof reading.entropyBits === "number" &&
		typeof reading.entropyThreshold === "number" &&
		reading.entropyThreshold > 0
			? reading.entropyBits / reading.entropyThreshold
			: 0;

	return {
		winner: winner.name,
		winnerPercent: `${Math.round(winnerProbability * 100)}%`,
		winnerBits: winnerBits.toFixed(2),
		runnerBits: runnerBits === null ? "—" : runnerBits.toFixed(2),
		kl: kl === null ? "—" : kl.toFixed(3),
		marginPercent:
			runnerBits === null
				? 0
				: Math.round(clamp((runnerBits - winnerBits) / 4, 0, 1) * 100),
		entropy:
			typeof reading?.entropyBits === "number"
				? reading.entropyBits.toFixed(2)
				: "0.00",
		entropyThreshold:
			typeof reading?.entropyThreshold === "number"
				? reading.entropyThreshold.toFixed(2)
				: "0.00",
		entropyPercent: Math.round(clamp(entropyRatio, 0, 1) * 100),
		ambiguous: reading?.ambiguous === true,
		classes: sorted.map((entry, index) => ({
			name: entry.name,
			percent: Math.round(entry.probability * 100),
			color: index === 0 ? "var(--acc)" : "var(--info)",
			foreground: index === 0 ? "var(--f1)" : "var(--f3)",
		})),
	};
};

export const cognitiveBeamsFromReading = (
	reading: Record<string, unknown> | null,
): CortexBeam[] => {
	const tree = cortexTreeFromReading(reading);

	if (tree === null) {
		return [];
	}

	return tree.beams;
};

export const cognitiveTreeFromReading = (
	reading: Record<string, unknown> | null,
): CortexTree | null => cortexTreeFromReading(reading);

export const drawCognitiveTree = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	reading: Record<string, unknown> | null,
): void => {
	const tree = cognitiveTreeFromReading(reading);

	if (tree === null) {
		context.clearRect(0, 0, width, height);
		context.fillStyle = "#938a7e";
		context.font = "11px JetBrains Mono, monospace";
		context.textAlign = "center";
		context.fillText("waiting for cognitive reading", width / 2, height / 2);
		context.textAlign = "left";

		return;
	}

	drawCortexTree(context, width, height, tree);
};
