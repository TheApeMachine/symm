import { beforeEach, describe, expect, it } from "vitest";

import { appStore } from "#/collections/app";
import { balanceStore } from "#/collections/balance";
import { signalStore } from "#/collections/signals";
import {
	decisionTreeBranches,
	finiteCount,
	gaugeFramesFromState,
	isPlaybookBranch,
	routeDecodedFrame,
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

describe("routeDecodedFrame", () => {
	beforeEach(() => {
		appStore.setState((previous) => ({
			...previous,
			storyTicks: 0,
			playbookEvaluations: 0,
			lastGaugeFrames: {},
		}));
		signalStore.setState({ readings: {} });
		balanceStore.setState((previous) => ({
			...previous,
			balanceLabel: "Balance",
			symbol: "$",
		}));
	});

	it("updates story tick counters", () => {
		routeDecodedFrame({
			type: "story_tick",
			story_ticks: 12,
			playbook_evaluations: 4,
		});

		expect(appStore.state.storyTicks).toBe(12);
		expect(appStore.state.playbookEvaluations).toBe(4);
	});

	it("hydrates gauges from measurement role frames", () => {
		routeDecodedFrame({
			role: "measurement",
			origin: "fluid",
			output: {
				confidence: 0.71,
				surprise: 2.1,
			},
			calibrated: true,
		});

		expect(appStore.state.lastGaugeFrames.fluid?.confidence).toBe(0.71);
		expect(signalStore.state.readings.fluid?.source).toBe("fluid");
	});

	it("hydrates visible signals from raw measurement artifacts", () => {
		routeDecodedFrame({
			role: "measurement",
			origin: "pumpdump",
			scope: "update",
			observed_at: 1766666666123,
		});

		expect(appStore.state.lastGaugeFrames.pumpdump?.source).toBe("pumpdump");
		expect(signalStore.state.readings.pumpdump?.source).toBe("pumpdump");
		expect(signalStore.state.readings.pumpdump?.observedAt).toBe(
			1766666666123,
		);
	});

	it("hydrates gauges from state gauge readings", () => {
		routeDecodedFrame({
			type: "state",
			playbook_evaluations: 7,
			gauge_readings: [
				{
					source: "pumpdump",
					output: {
						confidence: 0.67,
						value: 1.4,
					},
					calibrated: true,
				},
			],
			measurements: [
				{
					source: "fluid",
					output: {
						confidence: 0.2,
					},
					calibrated: true,
				},
			],
		});

		expect(appStore.state.lastGaugeFrames.pumpdump?.confidence).toBe(0.67);
		expect(signalStore.state.readings.pumpdump?.confidence).toBe(0.67);
		expect(appStore.state.lastGaugeFrames.fluid).toBeUndefined();
		expect(appStore.state.playbookEvaluations).toBe(7);
	});

	it("hydrates gauges from origin and output when role is ingest", () => {
		routeDecodedFrame({
			role: "ticker",
			origin: "pumpdump",
			output: {
				confidence: 0.42,
			},
		});

		expect(appStore.state.lastGaugeFrames.pumpdump?.confidence).toBe(0.42);
	});

	it("normalizes kraken wallet frames into header balance", () => {
		routeDecodedFrame({
			role: "wallet",
			type: "wallet",
			Currency: "USD",
			asset: [
				{
					asset: "USD",
					balance: 200,
				},
			],
		});

		expect(balanceStore.state.balanceLabel).toBe("$200.00");
	});
});
