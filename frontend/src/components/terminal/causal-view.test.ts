import { describe, expect, it } from "vitest";
import type { CausalFrame } from "#/collections/types";
import {
	causalAssociation,
	causalEntryBaseline,
	causalStrength,
} from "#/components/terminal/causal-view";

const frame = (reading: CausalFrame["reading"]): CausalFrame => ({
	source: "causal",
	symbol: "BTC/USD",
	at: "2026-07-12T00:00:00Z",
	reading,
});

describe("causal-view", () => {
	it("reads Pearl ladder values from the nested reading payload", () => {
		const sample = frame({
			strength: 0.61,
			entryBaseline: 0.31,
			association: 0.12,
			intervention: 0.22,
			noise: 0.03,
			contagion: 0.04,
		});

		expect(causalStrength(sample)).toBe(0.61);
		expect(causalEntryBaseline(sample)).toBe(0.31);
		expect(causalAssociation(sample)).toBe(0.12);
	});
});
