import { bench, describe } from "vitest";
import { Circular } from "#/collections/circular";
import { allocationSummary } from "./allocation-side";

const history = <T>(frame: T) => {
	const frames = Circular<T>(8);
	frames.push(frame);

	return frames;
};

const symbols = Array.from({ length: 24 }, (_, index) => `SYM${index}/USD`);
const causal = Object.fromEntries(
	symbols.map((symbol, index) => [
		symbol,
		history({
			source: "causal",
			symbol,
			at: "2026-07-06T10:00:00Z",
			reading: {
				strength: 0.2 + index / 40,
				entryBaseline: 0.25,
				confidence: 0.4,
			},
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
			balances: [
				{ asset: "USD", balance: 1200, available: 1000, reserved: 50 },
			],
			causal,
			manifold,
			positions: [],
			resonance,
		});
	});
});
