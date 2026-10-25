import { describe, expect, it } from "vitest";
import { Value } from "#/types/value";
import {
	cognitionStore,
	ingestPathWalks,
	ingestPolicyKeys,
	ingestS3Prefix,
	thresholdFromStored,
	ingestTrainRegions,
	parseTrainRegionsIndex,
	regionHistoryStore,
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

	it("leaves an unscored value without a threshold", () => {
		expect(thresholdFromStored({ Index: ["action"], Layers: [btoa(JSON.stringify("enter"))] })).toBeNull();
	});
});

describe("policy tree keys", () => {
	it("stores cognition/keys strings unchanged", () => {
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(
			[
				"model/explore/R01/R02/enter.bin",
				"model/explore/R01/R02/exit.bin",
			],
			"1",
		);
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/explore/R01/R02/enter.bin",
			"model/explore/R01/R02/exit.bin",
		]);
	});

	it("reads S3 keys from index suffixes after the frame prefix", () => {
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
		cognitionStore.setState((s) => ({ ...s, policyKeys: [], epoch: "0" }));
		ingestPolicyKeys(["model/explore/R01/R02/enter.bin"], "1");
		ingestPolicyKeys(["model/explore/R03/R04/exit.bin"], "2");
		expect(cognitionStore.state.policyKeys).toEqual([
			"model/explore/R03/R04/exit.bin",
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
