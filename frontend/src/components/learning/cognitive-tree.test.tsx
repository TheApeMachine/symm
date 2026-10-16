// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { Value } from "#/types/value";
import { keysHash, symbolProgress, tokensOf } from "./cognitive-tree";

describe("tokensOf", () => {
	it("rebuilds stored-key tokens from path_token metrics", () => {
		const m = new Value({
			metrics: Object.entries({
				"path_token:00:0": 1,
				"path_token:00:1": 2,
				"path_token:01:0": 11,
			}).map(([name, raw]) => ({
				name,
				raw,
			})),
		});
		expect(tokensOf(m)).toEqual(["R01-R02", "R11"]);
	});
});

describe("keysHash", () => {
	it("matches strategy.keysHash (FNV-1a 32)", () => {
		expect(keysHash(["a", "b"])).toBe(0x28e4c710);
	});
});

describe("symbolProgress", () => {
	it("tracks multiple symbols advancing through the trie", () => {
		const btc = new Value({
			scope: "BTC/USD",
			provenance: [
				{ name: "token_path", value: "R12/R11" },
				{ name: "token_depth", value: "2" },
			],
		});

		const eth = new Value({
			scope: "ETH/USD",
			provenance: [
				{ name: "token_path", value: "R05/R02/R01" },
				{ name: "token_depth", value: "3" },
			],
		});

		const state = {
			"BTC/USD": { toArray: () => [btc] },
			"ETH/USD": { toArray: () => [eth] },
		};

		const parsed = JSON.parse(symbolProgress(state));
		expect(parsed["BTC/USD"]).toBeDefined();
		expect(parsed["BTC/USD"].tokens).toEqual(["R12", "R11"]);
		expect(parsed["BTC/USD"].depth).toBe(2);

		expect(parsed["ETH/USD"]).toBeDefined();
		expect(parsed["ETH/USD"].tokens).toEqual(["R05", "R02", "R01"]);
		expect(parsed["ETH/USD"].depth).toBe(3);
	});
});
