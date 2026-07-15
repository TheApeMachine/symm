import { describe, expect, it } from "vitest";
import {
	type CognitiveReading,
	cognitiveReadingFromFrame,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";

describe("cognitive", () => {
	it("maps thesis cognition frames into per-symbol readings", () => {
		cognitiveStore.setState((prev) => ({ ...prev, readings: {} }));

		cognitiveStore.actions.updateFrame([
			{
				symbol: "BILL/USD",
				at: "2026-07-15T11:53:50.000Z",
				sequence: "symbol-bill-usd_pressure-positive",
				regimePrefix: "symbol-bill-usd",
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
				maxHops: 2,
				nodeCount: 3,
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
				beams: [
					{ sequence: "symbol-bill-usd_pressure-positive", score: -0.34 },
				],
				classes: [
					{ name: "buy", probability: 0.82 },
					{ name: "balanced", probability: 0.18 },
				],
			},
		]);

		expect(cognitiveStore.state.readings["BILL/USD"]).toMatchObject({
			scope: "BILL/USD",
			sequence: "symbol-bill-usd_pressure-positive",
			regimePrefix: "symbol-bill-usd",
			winnerClass: "buy",
			beamWidth: 2,
			maxHops: 2,
			nodeCount: 3,
			branches: expect.arrayContaining([
				expect.objectContaining({ token: "symbol-bill-usd" }),
			]),
		} satisfies Partial<CognitiveReading>);
	});

	it("maps one cognition object directly", () => {
		const reading = cognitiveReadingFromFrame({
			symbol: "BTC/USD",
			sequence: "symbol-btc-usd",
			winner: "sell",
			ready: false,
		});

		expect(reading).toMatchObject({
			scope: "BTC/USD",
			winnerClass: "sell",
			sideline: true,
		});
	});

	it("keeps cognitive scopes stable instead of confidence sorted", () => {
		const readings = {
			"ETH/USD": { scope: "ETH/USD" } as CognitiveReading,
			"BTC/USD": { scope: "BTC/USD" } as CognitiveReading,
			"ARB/USD": { scope: "ARB/USD" } as CognitiveReading,
		};

		expect(cognitiveScopes(readings)).toEqual([
			"ARB/USD",
			"BTC/USD",
			"ETH/USD",
		]);
	});
});
