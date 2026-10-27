import { describe, expect, it } from "vitest";
import { explorePathKept, filterExploreKeys } from "./explore-path";

describe("explorePathKept", () => {
	it("drops explore paths above the desk prune cutoff", () => {
		expect(
			explorePathKept("model/explore/R01/R02/enter.bin", {
				"model/explore/R01/R02/enter.bin": 0.81,
			}),
		).toBe(false);
		expect(
			explorePathKept("model/explore/R01/R02/enter.bin", {
				"model/explore/R01/R02/enter.bin": 0.8,
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
				[
					"model/explore/a/enter.bin",
					"model/explore/b/enter.bin",
				],
				{
					"model/explore/a/enter.bin": 0.55,
					"model/explore/b/enter.bin": 0.85,
				},
			),
		).toEqual(["model/explore/a/enter.bin"]);
	});
});
