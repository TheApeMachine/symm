import { describe, expect, it } from "vitest";

import {
	decisionTreeBranches,
	finiteCount,
	gaugeFramesFromState,
	isPlaybookBranch,
} from "#/providers/websocket-handlers";

describe("websocket frame handlers", () => {
	it.each([
		{ label: "undefined", value: undefined, expected: null },
		{ label: "negative", value: -1, expected: null },
		{ label: "NaN", value: Number.NaN, expected: null },
		{ label: "fraction", value: 3.9, expected: 3 },
		{ label: "integer", value: 12, expected: 12 },
	])("finiteCount handles $label", ({ value, expected }) => {
		expect(finiteCount(value)).toBe(expected);
	});

	it("extracts gauge frames from gauge_readings", () => {
		expect(
			gaugeFramesFromState({
				gauge_readings: [{ source: "fluid" }, null, { source: "toxicity" }],
			}),
		).toEqual([{ source: "fluid" }, { source: "toxicity" }]);
	});

	it("extracts measurement artifacts from the state frame", () => {
		expect(
			gaugeFramesFromState({
				measurements: [
					{
						origin: "fluid",
						scope: "BTC/USD",
						classifier: { confidence: 0.71, category: 2 },
					},
					null,
				],
			}),
		).toEqual([
			{
				origin: "fluid",
				scope: "BTC/USD",
				classifier: { confidence: 0.71, category: 2 },
			},
		]);
	});

	it("returns empty gauge frames when state has no arrays", () => {
		expect(gaugeFramesFromState({})).toEqual([]);
	});

	it("validates nested playbook branches", () => {
		expect(
			isPlaybookBranch({
				branches: [{ action: { type: "hold" } }],
			}),
		).toBe(true);
		expect(isPlaybookBranch({ branches: [null] })).toBe(false);
	});

	it("extracts top-level decision tree branches", () => {
		const branches = [{ action: { type: "hold" } }];

		expect(decisionTreeBranches({ branches })).toEqual(branches);
	});

	it("extracts nested decision tree branches", () => {
		const branches = [{ action: { type: "settle_position" } }];

		expect(decisionTreeBranches({ value: { branches } })).toEqual(branches);
	});

	it("returns null for malformed decision tree payloads", () => {
		expect(decisionTreeBranches({ branches: [null] })).toBeNull();
		expect(decisionTreeBranches({ value: "invalid" })).toBeNull();
	});
});
