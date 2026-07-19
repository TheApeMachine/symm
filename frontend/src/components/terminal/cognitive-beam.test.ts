import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/types";
import {
	cognitiveBeamModel,
	cognitiveReadingFor,
} from "#/components/terminal/cognitive-beam";

const reading = (overrides: Partial<CognitiveReading> = {}): CognitiveReading =>
	({
		symbol: "BONK/USD",
		regimeCohort: 3,
		sequence: "symbol-bonk · pump · hold",
		winnerClass: "pump",
		classConfidence: 0.82,
		lookaheadScore: 0.41,
		lookaheadPaths: 4,
		entropyBits: 0.2,
		entropyThreshold: 1,
		...overrides,
	}) as CognitiveReading;

describe("cognitiveReadingFor", () => {
	it("selects the concrete symbol when present", () => {
		const rows = [
			reading({ symbol: "AAVE/USD", winnerClass: "coil" }),
			reading(),
		];

		expect(cognitiveReadingFor(rows, "BONK/USD")?.winnerClass).toBe("pump");
	});
});

describe("cognitiveBeamModel", () => {
	it("maps finite cognitive fields into paint meters", () => {
		const model = cognitiveBeamModel(reading());

		expect(model?.cohort).toBe("3");
		expect(model?.winner).toBe("pump");
		expect(model?.paths).toBe("4");
		expect(model?.meters).toHaveLength(3);
		expect(model?.meters[1]?.value).toBe("82%");
		expect(model?.meters[1]?.percent).toBe(82);
	});

	it("returns null when no reading is available", () => {
		expect(cognitiveBeamModel(null)).toBeNull();
	});

	it("falls back when regimeCohort is missing", () => {
		const model = cognitiveBeamModel(reading({ regimeCohort: undefined }));

		expect(model?.cohort).toBe("—");
	});
});
