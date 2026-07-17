import { describe, expect, it } from "vitest";
import type { CausalFrame } from "#/collections/causal";
import type { StrategyDecision } from "#/types/thesis";
import { causalCleared, judgeCandidate } from "./decision-candidate";

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
