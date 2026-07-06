import { bench, describe } from "vitest";
import type { Action } from "#/collections/actions";
import { Circular } from "#/collections/circular";
import { allocationSummary } from "./allocation-side";

const history = <T,>(frame: T) => {
	const frames = Circular<T>(8);
	frames.push(frame);

	return frames;
};

const action = (symbol: string, score: number): Action => ({
	id: `1:${symbol}`,
	tick: 1,
	symbol,
	type: "entry",
	side: "buy",
	verdict: "allow",
	reason: "matched_branch",
	score,
	entryLine: 0,
	entryScore: score,
	entryConfidence: score,
	fraction: 0.05,
	price: 100,
	branchKey: "field/resonance/causal",
	reasonSource: "causal",
	reasonCategory: "edge",
	decisionAt: "2026-07-06T10:00:00Z",
});

const symbols = Array.from({ length: 24 }, (_, index) => `SYM${index}/USD`);
const actions = Object.fromEntries(
	symbols
		.filter((_, index) => index % 7 === 0)
		.map((symbol, index) => [symbol, history(action(symbol, 0.65 + index / 100))]),
);
const causal = Object.fromEntries(
	symbols.map((symbol, index) => [
		symbol,
		history({
			source: "causal",
			symbol,
			at: "2026-07-06T10:00:00Z",
			strength: 0.2 + index / 40,
			baseline: 0.25,
		}),
	]),
);
const manifold = Object.fromEntries(
	symbols.map((symbol) => [
		symbol,
		history({ source: "manifold", symbol, at: "2026-07-06T10:00:00Z" }),
	]),
);
const resonance = Object.fromEntries(
	symbols.map((symbol, index) => [
		symbol,
		history({
			source: "resonance",
			symbol,
			at: "2026-07-06T10:00:00Z",
			confidence: 0.3 + index / 50,
		}),
	]),
);

describe("allocationSummary", () => {
	bench("sizes a live allocation cross-section", () => {
		allocationSummary({
			focusSymbol: "BTC/USD",
			symbols,
			actions,
			balances: [{ asset: "USD", balance: 1200, available: 1000, reserved: 50 }],
			causal,
			manifold,
			positions: [],
			resonance,
		});
	});
});
