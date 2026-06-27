import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/cognitive";
import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
	cognitiveTreeFromReading,
} from "#/components/terminal/cognitive-viz";

const sampleReading = (): CognitiveReading => ({
	scope: "SOL/USD",
	sequence: "Z8RW-77JS-HM3K",
	regimePrefix: "breakout",
	regimeCohort: 4,
	ambiguous: false,
	sideline: false,
	entropyBits: 2.1,
	entropyThreshold: 3.6,
	classConfidence: 0.62,
	contrastEvidence: 0.18,
	lookaheadScore: 0.71,
	lookaheadPaths: 3,
	winnerClass: "breakout",
	prewarmPaths: null,
	prewarmScore: null,
	updatedAt: 0,
	beamWidth: 4,
	maxHops: 3,
	nodeCount: 6,
	branches: [
		{
			id: 0,
			parentId: -1,
			token: "•",
			prefix: "",
			depth: 0,
			probability: 1,
			count: 0,
		},
		{
			id: 1,
			parentId: 0,
			token: "Z8RW-77JS-HM3K",
			prefix: "Z8RW-77JS-HM3K",
			depth: 1,
			probability: 0.74,
			count: 6,
		},
		{
			id: 2,
			parentId: 1,
			token: "hold",
			prefix: "Z8RW-77JS-HM3K_hold",
			depth: 2,
			probability: 0.62,
			count: 3,
		},
		{
			id: 3,
			parentId: 2,
			token: "lift",
			prefix: "Z8RW-77JS-HM3K_hold_lift",
			depth: 3,
			probability: 0.55,
			count: 2,
		},
		{
			id: 4,
			parentId: 1,
			token: "thin",
			prefix: "Z8RW-77JS-HM3K_thin",
			depth: 2,
			probability: 0.38,
			count: 2,
		},
		{
			id: 5,
			parentId: 4,
			token: "ice",
			prefix: "Z8RW-77JS-HM3K_thin_ice",
			depth: 3,
			probability: 0.45,
			count: 1,
		},
	],
	beams: [
		{ sequence: "Z8RW-77JS-HM3K_hold_lift", score: -1.21 },
		{ sequence: "Z8RW-77JS-HM3K_thin_ice", score: -1.74 },
	],
	classes: [
		{ name: "breakout", probability: 0.62 },
		{ name: "hold", probability: 0.21 },
		{ name: "fade", probability: 0.17 },
	],
});

describe("cognitiveViz", () => {
	it("builds branching tree and four beams from backend cognitive reading", () => {
		const reading = sampleReading();
		const tree = cognitiveTreeFromReading(reading);
		const beams = cognitiveBeamsFromReading(reading);
		const posterior = cognitivePosteriorFromReading(reading);

		expect(tree?.beamWidth).toBe(4);
		expect(tree?.nodeCount).toBe(6);
		expect(tree?.maxDepth).toBe(3);
		expect(beams.length).toBe(2);
		expect(posterior.classes.length).toBeGreaterThan(1);
	});

	it("uses backend DMT branches instead of tokenizing a local tree", () => {
		const tree = cognitiveTreeFromReading({
			...sampleReading(),
			sequence: "BTC/USD_toxicity_measurement",
			branches: undefined,
		});

		expect(tree).toBeNull();
	});

	it("highlights the current sensory sequence, not the first lookahead beam", () => {
		const tree = cognitiveTreeFromReading({
			...sampleReading(),
			sequence: "Z8RW-77JS-HM3K_thin",
			beams: [
				{ sequence: "Z8RW-77JS-HM3K_hold_lift", score: -1.21 },
				{ sequence: "Z8RW-77JS-HM3K_thin_ice", score: -1.74 },
			],
		});

		expect(tree?.beamPrefixes.has("Z8RW-77JS-HM3K_thin")).toBe(true);
		expect(tree?.beamPrefixes.has("Z8RW-77JS-HM3K_hold")).toBe(false);
	});
});
