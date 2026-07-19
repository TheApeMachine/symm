import { describe, expect, it } from "vitest";
import type { CausalFrame } from "#/collections/types";
import type { ManifoldFrame } from "#/collections/types";
import type { ResonanceFrame } from "#/collections/types";
import type { StrategyDecision } from "#/types/thesis";
import {
	buildCandidate,
	causalCleared,
	judgeCandidate,
	manifoldField,
	resonanceEdge,
	resonancePredict,
} from "./decision-candidate";

const causalFrame = (
	strength: number | undefined,
	entryBaseline: number | undefined,
): CausalFrame =>
	({
		source: "causal",
		symbol: "BTC/USD",
		at: "2026-07-17T00:00:00Z",
		reading: {
			strength,
			entryBaseline,
		},
	}) as CausalFrame;

describe("causalCleared", () => {
	it("does not treat missing strength/baseline zeros as cleared", () => {
		expect(causalCleared(undefined)).toBe(false);
		expect(causalCleared(causalFrame(undefined, undefined))).toBe(false);
		expect(causalCleared(causalFrame(0, 0))).toBe(false);
		expect(causalCleared(causalFrame(1, 0))).toBe(true);
		expect(causalCleared(causalFrame(0.5, 0.5))).toBe(false);
	});
});

describe("judgeCandidate", () => {
	it("does not invent BLOCKED below utility without a strategy decision", () => {
		expect(
			judgeCandidate(undefined, 3, true, {
				causal: false,
				resonance: false,
				manifold: false,
			}),
		).toEqual({
			verdict: "waiting",
			why: "waiting strategy",
			inPlay: true,
		});

		expect(
			judgeCandidate(undefined, 3, false, {
				causal: false,
				resonance: false,
				manifold: false,
			}),
		).toEqual({
			verdict: "below",
			why: "below causal line",
			inPlay: false,
		});
	});

	it("surfaces the planner reason when strategy spoke", () => {
		const decision = {
			symbol: "BTC/USD",
			action: "nothing",
			utility: 0,
			reason: "expected executable return does not exceed doing nothing",
		} as StrategyDecision;

		expect(
			judgeCandidate(decision, 3, true, {
				causal: false,
				resonance: false,
				manifold: false,
			}),
		).toMatchObject({
			verdict: "hold",
			why: "expected executable return does not exceed doing nothing",
		});
	});
});

describe("resonancePredict", () => {
	it("maps surprise to analyzer forecast confidence contribution", () => {
		expect(resonancePredict(undefined)).toBeNull();
		expect(
			resonancePredict({
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				surprise: 0,
			} as ResonanceFrame),
		).toBe(1);
		expect(
			resonancePredict({
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				surprise: Math.LN2,
			} as ResonanceFrame),
		).toBeCloseTo(0.5, 10);
	});

	it("reads expectedReturn as the predict attribution edge", () => {
		expect(
			resonanceEdge({
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				expectedReturn: 0.012,
			} as ResonanceFrame),
		).toBe(0.012);
	});
});

describe("buildCandidate", () => {
	it("marks waiting when ladder frames are absent", () => {
		const model = buildCandidate(
			"BTC/USD",
			undefined,
			undefined,
			undefined,
			undefined,
		);

		expect(model).toMatchObject({
			symbol: "BTC/USD",
			support: 0,
			verdict: "waiting",
			why: "waiting causal",
			inPlay: false,
			hasDecision: false,
		});
		expect(model.bars).toEqual([]);
	});

	it("prefers strategy utility when a decision is present", () => {
		const decision = {
			symbol: "BTC/USD",
			action: "enter",
			utility: 0.82,
			reason: "edge clears",
			cause: "resonance+causal",
		} as StrategyDecision;

		const model = buildCandidate(
			"BTC/USD",
			decision,
			causalFrame(0.9, 0.2),
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				surprise: 0,
			} as ResonanceFrame,
			{
				source: "manifold",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				reading: { coherenceMag2: 0.5 },
			} as ManifoldFrame,
		);

		expect(model.score).toBe(0.82);
		expect(model.verdict).toBe("allow");
		expect(model.inPlay).toBe(true);
		expect(model.hasDecision).toBe(true);
		expect(model.support).toBe(3);
	});
});

describe("manifoldField", () => {
	it("reads coherenceMag2 from the nested reading", () => {
		expect(
			manifoldField({
				source: "manifold",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				reading: { coherenceMag2: 0.37 },
			} as ManifoldFrame),
		).toBe(0.37);
	});

	it("accepts legacy PascalCase reading keys", () => {
		expect(
			manifoldField({
				source: "manifold",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				reading: { CoherenceMag2: 0.41 },
			} as ManifoldFrame),
		).toBe(0.41);
	});

	it("does not invent a field from a missing reading", () => {
		expect(
			manifoldField({
				source: "manifold",
				symbol: "BTC/USD",
				at: "2026-07-18T00:00:00Z",
				momentum: 0.9,
			} as ManifoldFrame),
		).toBeNull();
	});
});
