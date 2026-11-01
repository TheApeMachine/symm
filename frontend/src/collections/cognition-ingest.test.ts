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
	thresholdFromStored,
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
	it("sums the threshold steps on top of the 0.50 start", () => {
		const encode = (value: unknown) => btoa(JSON.stringify(value));

		expect(
			thresholdFromStored({
				Index: [
					"model/explore/SYM/action",
					"model/explore/SYM/threshold",
					"model/explore/SYM/threshold",
				],
				Layers: [encode("enter"), encode(-0.05), encode(-0.05)],
			}),
		).toBeCloseTo(0.4);
	});

	it("sums resistance steps on top of the 0.50 start (backend key name)", () => {
		const encode = (value: unknown) => btoa(JSON.stringify(value));

		// The Go backend writes value.Append(-returnFraction, "resistance")
		// so the index is <prefix>/resistance
		expect(
			thresholdFromStored({
				Index: [
					"model/paths/R01/R02/resistance",
					"model/paths/R01/R02/resistance",
				],
				Layers: [encode(-0.05), encode(-0.03)],
			}),
		).toBeCloseTo(0.42);
	});

	it("treats a 0.05 resistance delta as a delta, not an absolute threshold", () => {
		const encode = (value: unknown) => btoa(JSON.stringify(value));

		// A 5% loss produces resistance = +0.05. This must be added to 0.50,
		// not treated as an absolute threshold.
		expect(
			thresholdFromStored({
				Index: ["model/paths/R01/R02/resistance"],
				Layers: [encode(0.05)],
			}),
		).toBeCloseTo(0.55);
	});

	it("reads a legacy absolute book before delta steps", () => {
		const encode = (value: unknown) => btoa(JSON.stringify(value));

		expect(
			thresholdFromStored({
				Index: ["model/explore/SYM/threshold", "model/explore/SYM/threshold"],
				Layers: [encode(0.49), encode(-0.01)],
			}),
		).toBeCloseTo(0.49);
	});

	it("keeps the last legacy snapshot instead of adding them", () => {
		const encode = (value: unknown) => btoa(JSON.stringify(value));

		expect(
			thresholdFromStored({
				Index: [
					"model/explore/SYM/threshold",
					"model/explore/SYM/threshold",
					"model/explore/SYM/threshold",
				],
				Layers: [encode(0.51), encode(0.52), encode(0.52)],
			}),
		).toBeCloseTo(0.51);
	});

	it("leaves an unscored value without a threshold", () => {
		expect(
			thresholdFromStored({
				Index: ["action"],
				Layers: [btoa(JSON.stringify("enter"))],
			}),
		).toBeNull();
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
