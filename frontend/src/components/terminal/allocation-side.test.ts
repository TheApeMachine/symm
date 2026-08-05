import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import { allocationSummary } from "./allocation-side";

const history = <T>(frame: T) => {
	const frames = Circular<T>(4);
	frames.push(frame);

	return frames;
};

const causalReading = (strength: number, entryBaseline: number) => ({
	strength,
	entryBaseline,
	confidence: strength,
});

describe("allocationSummary", () => {
	it("sizes the live ladder cross-section and keeps the instrument order stable", () => {
		const summary = allocationSummary({
			focusSymbol: "BTC/USD",
			symbols: ["BTC/USD", "ETH/USD", "SOL/USD"],
			balances: [
				{ asset: "USD", balance: 1200, available: 1000, reserved: 50 },
			],
			causal: {
				"BTC/USD": history({
					source: "causal",
					symbol: "BTC/USD",
					at: "1",
					reading: causalReading(0.2, 0.3),
				}),
				"ETH/USD": history({
					source: "causal",
					symbol: "ETH/USD",
					at: "1",
					reading: causalReading(0.6, 0.3),
				}),
				"SOL/USD": history({
					source: "causal",
					symbol: "SOL/USD",
					at: "1",
					reading: causalReading(0.95, 0.3),
				}),
			},
			manifold: {
				"BTC/USD": history({ source: "manifold", symbol: "BTC/USD", at: "1" }),
				"ETH/USD": history({ source: "manifold", symbol: "ETH/USD", at: "1" }),
				"SOL/USD": history({ source: "manifold", symbol: "SOL/USD", at: "1" }),
			},
			holdings: [
				{
					symbol: "BTC/USD",
					qty: 0.5,
					entry_price: 100,
					entry_fee: 0.13,
					exit_fee: 0.156,
					mark: 120,
					pnl: 10,
					return_pct: 0.2,
					is_opportunity: false,
				},
			],
			resonance: {
				"BTC/USD": history({
					source: "resonance",
					symbol: "BTC/USD",
					at: "1",
					surprise: -Math.log(0.2),
				}),
				"ETH/USD": history({
					source: "resonance",
					symbol: "ETH/USD",
					at: "1",
					surprise: -Math.log(0.6),
				}),
				"SOL/USD": history({
					source: "resonance",
					symbol: "SOL/USD",
					at: "1",
					surprise: -Math.log(0.95),
				}),
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
