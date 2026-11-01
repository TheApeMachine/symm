import { describe, expect, it } from "vitest";
import { explorePathKept, filterExploreKeys } from "./explore-path";

describe("explorePathKept", () => {
	it("drops explore paths above the desk prune cutoff", () => {
		expect(
			explorePathKept("model/paths/R01/R02/enter.bin", {
				"model/paths/R01/R02/enter.bin": 0.81,
			}),
		).toBe(false);
		expect(
			explorePathKept("model/paths/R01/R02/enter.bin", {
				"model/paths/R01/R02/enter.bin": 0.8,
			}),
		).toBe(true);
	});

	it("keeps non-explore keys regardless of thresholds", () => {
		expect(explorePathKept("policy/current/R01/R02/enter.bin", {})).toBe(
			true,
		);
	});

	it("filters a key list", () => {
		expect(
			filterExploreKeys(
				["model/paths/a/enter.bin", "model/paths/b/enter.bin"],
				{
					"model/paths/a/enter.bin": 0.55,
					"model/paths/b/enter.bin": 0.85,
				},
			),
		).toEqual(["model/paths/a/enter.bin"]);
	});
});
