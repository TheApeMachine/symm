import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/types";
import {
	cognitiveReadingFor,
	cognitiveScopes,
} from "#/components/terminal/cognitive-beam";

/*
These are the names the classifier publishes. The beam used to read
winnerClass / classConfidence / regimeCohort, none of which exist on the wire,
so pinning the real ones here is what keeps the panel bound to live data.
*/
const reading = (overrides: Partial<CognitiveReading> = {}): CognitiveReading =>
	({
		symbol: "BONK/USD",
		cohort: 3,
		sequence: "symbol-bonk · pump · hold",
		winner: "pump",
		candidateWinner: "pump",
		stateHeld: false,
		switchConfidence: 0.82,
		switchThreshold: 0.95,
		confidence: 0.82,
		contrast: 0.31,
		lookaheadScore: 0.41,
		lookaheadPaths: 4,
		entropyBits: 0.2,
		entropyThreshold: 1,
		...overrides,
	}) as CognitiveReading;

describe("cognitiveReadingFor", () => {
	it("selects the concrete symbol when present", () => {
		const rows = [reading({ symbol: "AAVE/USD", winner: "coil" }), reading()];

		expect(cognitiveReadingFor(rows, "BONK/USD")?.winner).toBe("pump");
	});

	it("reads a symbol-keyed map as readily as a batch", () => {
		const rows = {
			"AAVE/USD": reading({ symbol: "AAVE/USD", winner: "coil" }),
			"BONK/USD": reading(),
		};

		expect(cognitiveReadingFor(rows, "AAVE/USD")?.winner).toBe("coil");
	});

	it("falls back to the first scope that owns a reading", () => {
		const rows = [reading({ symbol: "AAVE/USD", winner: "coil" }), reading()];

		expect(cognitiveReadingFor(rows)?.symbol).toBe("AAVE/USD");
	});
});

describe("cognitiveScopes", () => {
	it("lists each symbol once, in order", () => {
		expect(
			cognitiveScopes([
				reading({ symbol: "SOL/USD" }),
				reading({ symbol: "AAVE/USD" }),
				reading({ symbol: "SOL/USD" }),
			]),
		).toEqual(["AAVE/USD", "SOL/USD"]);
	});
});
