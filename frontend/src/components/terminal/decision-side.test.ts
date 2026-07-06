import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/cognitive";
import type { MeasurementsState } from "#/collections/measurements";
import {
	causalReadingFor,
	cognitiveBeamModel,
	cognitiveReadingFor,
} from "#/components/terminal/decision-side";

const reading = (scope: string, confidence: number): CognitiveReading => ({
	scope,
	sequence: "Z7Z4-W3S-TX5M",
	regimePrefix: "trend-dn",
	regimeCohort: 9,
	ambiguous: false,
	sideline: false,
	entropyBits: 2.85,
	entropyThreshold: 3.6,
	classConfidence: confidence,
	contrastEvidence: 0.2,
	lookaheadScore: 0.316,
	lookaheadPaths: 14,
	winnerClass: "fade",
	updatedAt: 1,
});

describe("decision side rail", () => {
	it("selects the requested cognitive scope without borrowing another symbol", () => {
		const readings = {
			"OP/EUR": reading("OP/EUR", 0.31),
			"BTC/EUR": reading("BTC/EUR", 0.82),
		};

		expect(cognitiveReadingFor(readings, "OP/EUR")?.scope).toBe("OP/EUR");
		expect(cognitiveReadingFor(readings, "MISSING/EUR")).toBeNull();
		expect(cognitiveReadingFor(readings, "stream")?.scope).toBe("BTC/EUR");
	});

	it("formats the cognitive beam without synthetic values", () => {
		const model = cognitiveBeamModel(reading("OP/EUR", 0.31));

		expect(model).toMatchObject({
			cohort: "9",
			sequence: "Z7Z4-W3S-TX5M",
			winner: "fade",
			paths: "14",
		});
		expect(model?.meters.map((meter) => meter.value)).toEqual([
			"2.85 / 3.6 bits",
			"31%",
			"0.316",
		]);
	});

	it("selects causal measurements from the measurement store shape", () => {
		const latest = { source: "causal", symbol: "BTC/EUR", metrics: { beta: 0.4 } };
		const readings = {
			measurements: {
				causal: {
					push: () => undefined,
					values: () => [latest],
				},
			},
			symbols: {
				"OP/EUR": [{ source: "causal", symbol: "OP/EUR", metrics: { beta: 0.7 } }],
				"BTC/EUR": [latest],
			},
			sources: new Set(["causal"]),
			tick: 1,
		} as unknown as MeasurementsState;

		expect(causalReadingFor(readings, "causal", "OP/EUR")?.symbol).toBe(
			"OP/EUR",
		);
		expect(causalReadingFor(readings, "causal", "MISSING/EUR")).toBeUndefined();
		expect(causalReadingFor(readings, "causal", "stream")).toBe(latest);
	});
});
