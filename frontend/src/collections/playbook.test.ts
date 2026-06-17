import { describe, expect, it } from "vitest";

import {
	mergeWalkActivation,
	parseWalkTrace,
	persistedNodeState,
} from "#/collections/playbook";

describe("parseWalkTrace", () => {
	it("returns null when walk payload is absent", () => {
		expect(parseWalkTrace(undefined)).toBeNull();
		expect(parseWalkTrace(null)).toBeNull();
	});

	it("parses a valid walk trace", () => {
		expect(
			parseWalkTrace({
				symbol: "BTC/USD",
				steps: [{ path: [0], outcome: "matched" }],
			}),
		).toEqual({
			symbol: "BTC/USD",
			steps: [{ path: [0], outcome: "matched" }],
			active_path: undefined,
		});
	});
});

describe("mergeWalkActivation", () => {
	it("keeps matched nodes green across later walks", () => {
		const activated = mergeWalkActivation(
			{},
			{
				symbol: "BTC/USD",
				steps: [
					{ path: [0], outcome: "matched" },
					{ path: [0, 1], outcome: "matched" },
				],
			},
		);

		expect(activated).toEqual({
			"0": "matched",
			"0.1": "matched",
		});

		const nextWalk = mergeWalkActivation(activated, {
			symbol: "ETH/USD",
			steps: [{ path: [0], outcome: "rejected" }],
		});

		expect(nextWalk).toEqual({
			"0": "matched",
			"0.1": "matched",
		});
	});

	it("promotes action nodes above matched history", () => {
		const activated = mergeWalkActivation(
			{ "0.1.2": "matched" },
			{
				symbol: "BTC/USD",
				steps: [{ path: [0, 1, 2], outcome: "action" }],
			},
		);

		expect(activated["0.1.2"]).toBe("action");
	});
});

describe("persistedNodeState", () => {
	it("returns matched for previously activated paths without a live step", () => {
		expect(
			persistedNodeState(
				[0, 1],
				{
					symbol: "BTC/USD",
					steps: [],
				},
				{ "0.1": "matched" },
			),
		).toBe("matched");
	});
});
