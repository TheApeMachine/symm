import { describe, expect, it } from "vitest";
import type { Action } from "#/collections/actions";
import { Circular } from "#/collections/circular";
import { allocationSummary } from "./allocation-side";

const action = (symbol: string, entryScore: number): Action => ({
	id: `1:${symbol}`,
	tick: 1,
	symbol,
	type: "entry",
	side: "buy",
	verdict: "allow",
	reason: "matched_branch",
	score: entryScore,
	entryLine: 0,
	entryScore,
	entryConfidence: 1,
	fraction: 0.05,
	price: 100,
	branchKey: "field/resonance/causal",
	reasonSource: "causal",
	reasonCategory: "edge",
	decisionAt: "2026-07-06T10:00:00Z",
});

const history = <T,>(frame: T) => {
	const frames = Circular<T>(4);
	frames.push(frame);

	return frames;
};

describe("allocationSummary", () => {
	it("sizes the live ladder cross-section and keeps the instrument order stable", () => {
		const summary = allocationSummary({
			focusSymbol: "BTC/USD",
			symbols: ["BTC/USD", "ETH/USD", "SOL/USD"],
			actions: {
				"SOL/USD": history(action("SOL/USD", 0.95)),
			},
			balances: [{ asset: "USD", balance: 1200, available: 1000, reserved: 50 }],
			causal: {
				"BTC/USD": history({ source: "causal", symbol: "BTC/USD", at: "1", strength: 0.2, baseline: 0.3 }),
				"ETH/USD": history({ source: "causal", symbol: "ETH/USD", at: "1", strength: 0.6, baseline: 0.3 }),
				"SOL/USD": history({ source: "causal", symbol: "SOL/USD", at: "1", strength: 0.95, baseline: 0.3 }),
			},
			manifold: {
				"BTC/USD": history({ source: "manifold", symbol: "BTC/USD", at: "1" }),
				"ETH/USD": history({ source: "manifold", symbol: "ETH/USD", at: "1" }),
				"SOL/USD": history({ source: "manifold", symbol: "SOL/USD", at: "1" }),
			},
			positions: [
				{ symbol: "BTC/USD", qty: 0.5, entry_price: 100, mark: 120, pnl: 10, return_pct: 0.2 },
			],
			resonance: {
				"BTC/USD": history({ source: "resonance", symbol: "BTC/USD", at: "1", confidence: 0.2 }),
				"ETH/USD": history({ source: "resonance", symbol: "ETH/USD", at: "1", confidence: 0.6 }),
				"SOL/USD": history({ source: "resonance", symbol: "SOL/USD", at: "1", confidence: 0.95 }),
			},
		});

		expect(summary.rows.map((row) => row.symbol)).toEqual([
			"BTC/USD",
			"ETH/USD",
			"SOL/USD",
		]);
		expect(summary.deployable).toBe(1000);
		expect(summary.deployed).toBe(60);
		expect(summary.reserved).toBe(50);
		expect(summary.rows[1]?.inPlay).toBe(true);
		expect(summary.rows[2]?.allocated).toBe(true);
		expect(summary.rows[2]?.notional).toBeGreaterThan(0);
	});
});
