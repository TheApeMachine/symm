import { describe, expect, it } from "vitest";
import { bookAtom } from "#/collections/book";
import { Value } from "#/types/value";
import {
	cognitionStore,
	ingestPathWalks,
	ingestPolicyKeys,
	ingestS3Prefix,
	ingestTrainRegions,
	parseTrainRegionsIndex,
	regionHistoryStore,
	thresholdFromStored,
} from "./cognition";

const enc = new TextEncoder();

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
		).toBeCloseTo(0.48);
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
		).toBeCloseTo(0.52);
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
	it("stores cognition/keys strings unchanged", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(
			["model/explore/R01/R02/enter.bin", "model/explore/R01/R02/exit.bin"],
			"1",
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/explore/R01/R02/enter.bin",
			"model/explore/R01/R02/exit.bin",
		]);
	});

	it("reads S3 keys from index suffixes after the frame prefix", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		const framePrefix =
			"s3/prefix/model/explore/0000000000000000001/0000000000000000002";
		const layer = enc.encode(JSON.stringify("enter"));
		ingestS3Prefix(
			new Value({
				origin: "s3",
				role: "prefix",
				scope: "model/explore",
				prefix: framePrefix,
				epoch: 2n,
				index: [
					`${framePrefix}/model/explore/R01/R02/enter.bin`,
					`${framePrefix}/model/explore/R01/R02/exit.bin`,
					`${framePrefix}/model/explore/custom/path/enter.bin`,
				],
				layers: [layer, layer, layer],
			}),
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/explore/R01/R02/enter.bin",
			"model/explore/R01/R02/exit.bin",
			"model/explore/custom/path/enter.bin",
		]);
		expect(cognitionStore.state.epoch).toBe("2");
		expect(cognitionStore.state.thresholds).toEqual({});
	});

	it("reads a threshold off the stored value body", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		const framePrefix = "s3/prefix/model/explore/1/2";
		const key = "model/explore/R01/R02/exit.bin";
		const stored = {
			Index: [`${key}/threshold`],
			Layers: [btoa(JSON.stringify(-0.05))],
		};
		ingestS3Prefix(
			new Value({
				origin: "s3",
				role: "prefix",
				scope: "model/explore",
				prefix: framePrefix,
				epoch: 3n,
				index: [`${framePrefix}/${key}`],
				layers: [enc.encode(JSON.stringify(stored))],
			}),
		);
		expect(cognitionStore.state.thresholds[key]).toBeCloseTo(0.45);
	});

	it("ignores s3 frames for other prefixes", () => {
		cognitionStore.setState((s) => ({
			...s,
			policyKeys: ["keep"],
			epoch: "0",
		}));
		ingestS3Prefix(
			new Value({
				origin: "s3",
				role: "prefix",
				scope: "other/prefix",
				prefix: "s3/prefix/other/prefix/1/2",
				index: ["s3/prefix/other/prefix/1/2/foo/bar.bin"],
				layers: [enc.encode("{}")],
			}),
		);
		expect(cognitionStore.state.policyKeys).toEqual(["keep"]);
	});

	it("replaces the list on the next publish", () => {
		bookAtom.set("virtual");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(["model/explore/R01/R02/enter.bin"], "1");
		ingestPolicyKeys(["model/explore/R03/R04/exit.bin"], "2");
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/explore/R03/R04/exit.bin",
		]);
	});

	it("keeps the paper trie when the virtual catalog arrives", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		const framePrefix = "s3/prefix/model/policy/1/2";
		const layer = enc.encode(JSON.stringify("enter"));
		ingestS3Prefix(
			new Value({
				origin: "s3",
				role: "prefix",
				scope: "model/policy",
				prefix: framePrefix,
				epoch: 4n,
				index: [`${framePrefix}/model/policy/R08.R01/R07.R00/enter.bin`],
				layers: [layer],
			}),
		);
		ingestS3Prefix(
			new Value({
				origin: "s3",
				role: "prefix",
				scope: "model/explore",
				prefix: "s3/prefix/model/explore/1/2",
				epoch: 5n,
				index: [
					"s3/prefix/model/explore/1/2/model/explore/R01/R02/enter.bin",
				],
				layers: [layer],
			}),
		);
		expect(ingestPolicyKeys(["model/explore/R01/R02/enter.bin"], "6")).toBe(
			false,
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/policy/R08.R01/R07.R00/enter.bin",
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
