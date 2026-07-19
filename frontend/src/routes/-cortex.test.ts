import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/types";
import { activeScopeFor, cognitiveScopes } from "#/routes/cortex";

const reading = (scope: string, classConfidence: number): CognitiveReading => ({
	symbol: scope,
	scope,
	sequence: "",
	regimePrefix: "",
	regimeCohort: 0,
	ambiguous: false,
	sideline: false,
	entropyBits: 0,
	entropyThreshold: 0,
	classConfidence,
	contrastEvidence: 0,
	lookaheadScore: 0,
	lookaheadPaths: 0,
	winnerClass: "",
	updatedAt: 0,
});

describe("cortex", () => {
	it("uses app focus before selected scope", () => {
		const scopes = ["BTC/USD", "ETH/USD"];

		expect(activeScopeFor(new Set(scopes), "BTC/USD", scopes)).toBe(
			"BTC/USD",
		);
	});

	it("falls back when focus has no cognitive reading yet", () => {
		const scopes = ["ETH/USD"];

		expect(activeScopeFor(new Set(scopes), "BTC/USD", scopes)).toBe(
			"ETH/USD",
		);
	});

	it("keeps cognitive scopes stable instead of confidence sorted", () => {
		const readings = {
			"ETH/USD": reading("ETH/USD", 0.9),
			"BTC/USD": reading("BTC/USD", 0.2),
			"ARB/USD": reading("ARB/USD", 0.5),
		};

		expect(cognitiveScopes(readings)).toEqual([
			"ARB/USD",
			"BTC/USD",
			"ETH/USD",
		]);
	});
});
