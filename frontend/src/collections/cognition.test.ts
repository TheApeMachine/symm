import { describe, expect, it } from "vitest";
import { bookAtom } from "#/collections/book";
import { cognitionStore, ingestExploreListPrefix } from "./cognition";

describe("ingestExploreListPrefix", () => {
	it("ignores listings for another book scope", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({
			...s,
			policyKeys: ["keep"],
			epoch: "0",
		}));

		ingestExploreListPrefix({
			scope: "model/explore",
			epoch: "1",
			policyKeys: ["model/explore/R01/enter.bin"],
			thresholds: { "model/explore/R01/enter.bin": 0.5 },
		});

		expect(cognitionStore.state.policyKeys).toEqual(["keep"]);
	});

	it("merges chunked pathbook frames for the active book", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));

		ingestExploreListPrefix({
			scope: "model/paths",
			epoch: "2",
			policyKeys: [
				"model/paths/R01/R02/enter.bin",
				"model/paths/R01/R02/exit.bin",
			],
			thresholds: {
				"model/paths/R01/R02/enter.bin": 0.5,
				"model/paths/R01/R02/exit.bin": 0.5,
			},
		});
		ingestExploreListPrefix({
			scope: "model/paths",
			epoch: "2",
			policyKeys: ["model/paths/custom/path/enter.bin"],
			thresholds: {
				"model/paths/custom/path/enter.bin": 0.5,
			},
		});

		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R01/R02/enter.bin",
			"model/paths/R01/R02/exit.bin",
			"model/paths/custom/path/enter.bin",
		]);
		expect(cognitionStore.state.epoch).toBe("2");
	});

	it("prunes keys above the desk cutoff", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));

		ingestExploreListPrefix({
			scope: "model/paths",
			epoch: "3",
			policyKeys: [
				"model/paths/R01/R02/exit.bin",
				"model/paths/R03/R04/exit.bin",
			],
			thresholds: {
				"model/paths/R01/R02/exit.bin": 0.45,
				"model/paths/R03/R04/exit.bin": 0.9,
			},
		});

		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R01/R02/exit.bin",
		]);
	});
});