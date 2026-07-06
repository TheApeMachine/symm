import { describe, expect, it } from "vitest";
import {
	type CognitiveReading,
	cognitiveScopes,
} from "#/collections/cognitive";
import { activeScopeFor } from "#/routes/cortex";

const reading = (
	scope: string,
	classConfidence: number,
): CognitiveReading => ({
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
		const readings = {
			"ETH/USD": reading("ETH/USD", 0.9),
			"BTC/USD": reading("BTC/USD", 0.2),
		};

		expect(
			activeScopeFor(
				readings,
				"ETH/USD",
				"BTC/USD",
				["BTC/USD", "ETH/USD"],
			),
		).toBe("BTC/USD");
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
