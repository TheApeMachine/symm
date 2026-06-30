import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/cognitive";
import { activeScopeFor } from "#/routes/cortex";

const reading = (scope: string): CognitiveReading => ({
	scope,
	sequence: "Z7Z4",
	regimePrefix: "trend",
	regimeCohort: 1,
	ambiguous: false,
	sideline: false,
	entropyBits: 1,
	entropyThreshold: 2,
	classConfidence: 0.5,
	contrastEvidence: 0.1,
	lookaheadScore: 0.2,
	lookaheadPaths: 3,
	winnerClass: "hold",
	updatedAt: 1,
});

describe("cortex route model", () => {
	it("does not borrow another scope for a missing concrete focus", () => {
		const readings = { "ETH/USD": reading("ETH/USD") };

		expect(activeScopeFor(readings, null, "BTC/USD", ["ETH/USD"])).toBeNull();
	});

	it("falls back to the first scope only in stream mode", () => {
		const readings = { "ETH/USD": reading("ETH/USD") };

		expect(activeScopeFor(readings, null, "stream", ["ETH/USD"])).toBe(
			"ETH/USD",
		);
	});
});
