import { describe, expect, it } from "vitest";
import { bookAtom } from "#/collections/book";
import { applyPathsEnvelope } from "#/hub/apply-s3-compact";
import {
	cognitionStore,
	ingestExploreListPrefix,
	ingestPathWalks,
	ingestPolicyKeys,
	ingestTrainRegions,
	parseTrainRegionsIndex,
	regionHistoryStore,
	thresholdFromExploreBytes,
	thresholdFromPathBook,
} from "./cognition";

/*
The backend emits one layer per region at
<prefix>/train/regions/<SYM>/<R##> with [brightness], and the live sentence
at <prefix>/train/regions/<SYM>/tokens with ["R01", ...]. These are exactly
those two shapes from a live BTC/USD frame.
*/
const PREFIX = "workspace/tick/aggregate/1792663234/0000000000000000075";

describe("path walks", () => {
	it("keeps one sentence per symbol from the tick batch", () => {
		cognitionStore.setState((s) => ({ ...s, matches: {} }));

		expect(
			ingestPathWalks(9, [
				{
					symbol: "BTC/USD",
					phase: "R08.R03/R08.R05",
					policy: "R08.R03/R08.R05",
					enter: false,
				},
				{ symbol: "", phase: "R07.R01" },
			]),
		).toBe(true);

		const match = cognitionStore.state.matches["BTC/USD"];
		expect(match?.tokens).toEqual(["R08.R03", "R08.R05"]);
		expect(match?.depth).toBe(2);
		expect(match?.action).toBe("exit");
		expect(cognitionStore.state.matches[""]).toBeUndefined();
	});

	it("uses storyPhase hold for trie matching", () => {
		cognitionStore.setState((s) => ({ ...s, matches: {} }));

		ingestPathWalks(10, [
			{
				symbol: "ETH/USD",
				phase: "R11.R08/R07.R05",
				policy: "R11.R08/R07.R05",
				storyPhase: "hold",
				enter: false,
			},
		]);

		expect(cognitionStore.state.matches["ETH/USD"]?.action).toBe("hold");
	});
});

describe("stored thresholds", () => {
	it("reads PathBook JSON absolute resistance", () => {
		expect(
			thresholdFromPathBook({
				action: "enter",
				resistance: 0.45,
			}),
		).toBeCloseTo(0.45);
	});

	it("reads PathBook bytes and hub compact floats", () => {
		const book = new TextEncoder().encode(
			JSON.stringify({ action: "exit", resistance: 0.37 }),
		);
		expect(thresholdFromExploreBytes(book)).toBeCloseTo(0.37);
		expect(
			thresholdFromExploreBytes(new TextEncoder().encode("0.55")),
		).toBeCloseTo(0.55);
	});

	it("rejects non-PathBook payloads", () => {
		expect(thresholdFromPathBook({ Index: ["action"], Layers: [] })).toBeNull();
		expect(thresholdFromPathBook({ action: "enter" })).toBeNull();
		expect(thresholdFromPathBook(null)).toBeNull();
	});
});

describe("policy tree keys", () => {
	it("ingests hub paths envelopes into the trie book", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));

		expect(
			applyPathsEnvelope({
				scope: "model/paths",
				epoch: "4",
				thresholds: {
					"model/paths/R01/R02/enter.bin": 0.5,
				},
			}),
		).toBe(true);

		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R01/R02/enter.bin",
		]);
		expect(cognitionStore.state.epoch).toBe("4");
	});

	it("stores cognition/keys strings unchanged", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(
			["model/paths/R01/R02/enter.bin", "model/paths/R01/R02/exit.bin"],
			"1",
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R01/R02/enter.bin",
			"model/paths/R01/R02/exit.bin",
		]);
	});

	it("merges chunked paths envelopes", () => {
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

	it("prunes paths above the desk cutoff", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));

		applyPathsEnvelope({
			scope: "model/paths",
			epoch: "3",
			thresholds: {
				"model/paths/R01/R02/exit.bin": 0.45,
				"model/paths/R03/R04/exit.bin": 0.9,
			},
		});

		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R01/R02/exit.bin",
		]);
		expect(cognitionStore.state.thresholds["model/paths/R01/R02/exit.bin"]).toBe(
			0.45,
		);
	});

	it("ignores paths envelopes for other scopes", () => {
		cognitionStore.setState((s) => ({
			...s,
			policyKeys: ["keep"],
			epoch: "0",
		}));

		expect(
			applyPathsEnvelope({
				scope: "other/prefix",
				thresholds: { "other/prefix/foo.bin": 0.5 },
			}),
		).toBe(false);
		expect(cognitionStore.state.policyKeys).toEqual(["keep"]);
	});

	it("replaces the list on the next publish", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(["model/paths/R01/R02/enter.bin"], "1");
		ingestPolicyKeys(["model/paths/R03/R04/exit.bin"], "2");
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R03/R04/exit.bin",
		]);
	});

	it("keeps the listed trie when a stale catalog arrives", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));

		ingestExploreListPrefix({
			scope: "model/paths",
			epoch: "4",
			policyKeys: ["model/paths/R08.R01/R07.R00/enter.bin"],
			thresholds: {
				"model/paths/R08.R01/R07.R00/enter.bin": 0.5,
			},
		});

		expect(ingestPolicyKeys(["model/explore/R01/R02/enter.bin"], "6")).toBe(
			false,
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/paths/R08.R01/R07.R00/enter.bin",
		]);
	});
});

describe("training regions ingest", () => {
	it("feeds the ring from a region layer", () => {
		const ok = ingestTrainRegions(
			75,
			`${PREFIX}/train/regions/BTC/USD/R01`,
			[0.5],
		);
		expect(ok).toBe(true);
		const rings = regionHistoryStore.state["BTC/USD"];
		expect(rings).toBeDefined();
		expect(rings[rings.length - 1][1]).toBe(0.5);
	});

	it("feeds the match from the sentence layer", () => {
		ingestTrainRegions(75, `${PREFIX}/train/regions/BTC/USD/tokens`, [
			"R01",
			"R02",
			"R03",
		]);
		const match = cognitionStore.state.matches["BTC/USD"];
		expect(match.tokens).toEqual(["R01", "R02", "R03"]);
		expect(match.depth).toBe(3);
	});

	it("parses both index shapes", () => {
		expect(
			parseTrainRegionsIndex(`${PREFIX}/train/regions/BTC/USD/R07`),
		).toEqual({ symbol: "BTC/USD", tokens: ["R07"] });
		expect(
			parseTrainRegionsIndex(`${PREFIX}/train/regions/BTC/USD/tokens`),
		).toEqual({ symbol: "BTC/USD", tokens: [] });
	});
});
