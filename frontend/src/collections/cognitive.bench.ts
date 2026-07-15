import { bench } from "vitest";
import { cognitiveReadingFromFrame } from "#/collections/cognitive";

const cognitionFrame = {
	symbol: "BILL/USD",
	at: "2026-07-15T11:53:50.000Z",
	sequence: "symbol-bill-usd_pressure-positive_stress-zero",
	regimePrefix: "symbol-bill-usd_pressure-positive",
	winner: "buy",
	ready: true,
	confidence: 0.82,
	contrast: 0.14,
	entropyBits: 0.42,
	entropyThreshold: 0.91,
	ambiguous: false,
	cohort: 3,
	lookaheadScore: 0.76,
	lookaheadPaths: 2,
	beamWidth: 2,
	maxHops: 3,
	nodeCount: 4,
	branches: [
		{
			id: 0,
			parentId: -1,
			token: "•",
			prefix: "",
			depth: 0,
			probability: 1,
			count: 1,
		},
		{
			id: 1,
			parentId: 0,
			token: "symbol-bill-usd",
			prefix: "symbol-bill-usd",
			depth: 1,
			probability: 0.71,
			count: 2,
		},
	],
	beams: [{ sequence: "symbol-bill-usd_pressure-positive", score: -0.34 }],
	classes: [
		{ name: "buy", probability: 0.82 },
		{ name: "balanced", probability: 0.18 },
	],
};

bench("cognitiveReadingFromFrame", () => {
	cognitiveReadingFromFrame(cognitionFrame);
});
