import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { actionStore, normalizeActions } from "./actions";

beforeEach(() => {
	vi.spyOn(console, "warn").mockImplementation(() => {});
	actionStore.actions.reset();
});

afterEach(() => {
	vi.restoreAllMocks();
	actionStore.actions.reset();
});

describe("normalizeActions", () => {
	it("accepts planner intents with backend-owned field casing", () => {
		const [action] = normalizeActions([
			{
				Symbol: "ETH/USD",
				Action: "buy",
				Size: "0.05",
				Edge: 0.017,
				Velocity: 0.018,
				Confidence: 0.82,
				Thesis: {},
			},
		]);

		expect(action).toMatchObject({
			symbol: "ETH/USD",
			side: "buy",
			type: "intent",
			verdict: "allow",
			reason: "planner_intent",
			score: 0.017,
			entryScore: 0.017,
			entryConfidence: 0.82,
			fraction: 0.05,
			reasonSource: "planner",
			reasonCategory: "buy",
		});
	});

	it("keeps legacy action frames usable by the existing panels", () => {
		const [action] = normalizeActions([
			{
				id: "1:BTC/USD",
				tick: 1,
				symbol: "BTC/USD",
				type: "entry",
				side: "buy",
				verdict: "allow",
				reason: "matched_branch",
				score: 0.7,
				entryLine: 0.4,
				entryScore: 0.7,
				entryConfidence: 0.9,
				fraction: 0.1,
				price: 100,
				branchKey: "field/resonance/causal",
				reasonSource: "causal",
				reasonCategory: "edge",
				decisionAt: "2026-07-06T10:00:00Z",
			},
		]);

		expect(action).toMatchObject({
			id: "1:BTC/USD",
			tick: 1,
			symbol: "BTC/USD",
			type: "entry",
			side: "buy",
			verdict: "allow",
			reason: "matched_branch",
			score: 0.7,
			entryLine: 0.4,
			entryScore: 0.7,
			entryConfidence: 0.9,
			fraction: 0.1,
			price: 100,
			branchKey: "field/resonance/causal",
			reasonSource: "causal",
			reasonCategory: "edge",
			decisionAt: "2026-07-06T10:00:00Z",
		});
	});
});

describe("actionStore", () => {
	it("indexes planner intents by symbol", () => {
		actionStore.actions.updateFrame([
			{
				Symbol: "SOL/USD",
				Action: "sell",
				Size: "0",
				Edge: 0.021,
				Velocity: 0.021,
				Confidence: 0.76,
				Thesis: {},
			},
		]);

		expect(actionStore.state.actions["SOL/USD"]?.values()).toMatchObject([
			{
				symbol: "SOL/USD",
				side: "sell",
				verdict: "allow",
				score: 0.021,
				entryConfidence: 0.76,
			},
		]);
	});
});
