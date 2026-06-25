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
});

describe("cognitiveViz", () => {
	it("builds branching tree and four beams from backend cognitive reading", () => {
		const reading = sampleReading();
		const tree = cognitiveTreeFromReading(reading);
		const beams = cognitiveBeamsFromReading(reading);
		const posterior = cognitivePosteriorFromReading(reading);

		expect(tree?.beamWidth).toBe(4);
		expect(tree?.nodeCount).toBeGreaterThan(4);
		expect(tree?.maxDepth).toBe(3);
		expect(beams.length).toBe(4);
		expect(posterior.classes.length).toBeGreaterThan(1);
	});

	it("tokenizes underscore dmt sequences into a branching cortex tree", () => {
		const tree = cognitiveTreeFromReading({
			...sampleReading(),
			sequence: "BTC/USD_toxicity_measurement",
		});

		expect(tree?.beamWidth).toBe(4);
		expect(tree?.nodeCount).toBeGreaterThan(4);
		expect(tree?.maxDepth).toBe(3);
	});
});
