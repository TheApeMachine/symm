import { afterEach, describe, expect, it, vi } from "vitest";
import type { Measurement } from "#/types/measurement";
import {
	type MeterParts,
	mergeInspectorMetrics,
	paintInspectorMeters,
} from "./inspector-meters";

type FakeNode = {
	dataset: Record<string, string>;
	style: Record<string, string>;
	textContent: string;
	removed: boolean;
	children: FakeNode[];
	setAttribute: (name: string, value: string) => void;
	append: (...nodes: FakeNode[]) => void;
	remove: () => void;
};

const fakeNode = (): FakeNode => {
	const node: FakeNode = {
		dataset: {},
		style: {},
		textContent: "",
		removed: false,
		children: [],
		setAttribute: () => undefined,
		append: (...nodes: FakeNode[]) => {
			node.children.push(...nodes);
		},
		remove: () => {
			node.removed = true;
		},
	};

	return node;
};

const measurement = (
	at: string,
	metrics: Record<string, number>,
): Measurement => ({
	source: "hawkes",
	symbol: "BTC/USD",
	at,
	raw: 0,
	normalized: null,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: "", through: "" },
	metrics,
});

describe("mergeInspectorMetrics", () => {
	it("unions live and fit epochs for the inspected source", () => {
		const merged = mergeInspectorMetrics(
			[
				measurement("2026-07-25T01:00:00.000Z", {
					"conditional_intensity:buy": 0.18,
					"conditional_intensity:sell": 0.21,
				}),
				measurement("2026-07-25T00:59:00.000Z", {
					decay_rate: 1.9,
					spectral_radius: 0.32,
				}),
			],
			"hawkes",
			"BTC/USD",
		);

		expect(Object.fromEntries(merged)).toEqual({
			"conditional_intensity:buy": 0.18,
			"conditional_intensity:sell": 0.21,
			decay_rate: 1.9,
			spectral_radius: 0.32,
		});
	});

	it("keeps later values when the same metric arrives twice", () => {
		const merged = mergeInspectorMetrics(
			[
				measurement("2026-07-25T01:00:00.000Z", { decay_rate: 1.9 }),
				measurement("2026-07-25T01:00:01.000Z", { decay_rate: 2.1 }),
			],
			"hawkes",
			"BTC/USD",
		);

		expect(merged.get("decay_rate")).toBe(2.1);
	});

	it("ignores other sources and off-focus symbols", () => {
		const merged = mergeInspectorMetrics(
			[
				{
					...measurement("2026-07-25T01:00:00.000Z", { decay_rate: 1.9 }),
					source: "pumpdump",
				},
				{
					...measurement("2026-07-25T01:00:00.000Z", {
						spectral_radius: 0.32,
					}),
					symbol: "ETH/USD",
				},
				measurement("2026-07-25T01:00:00.000Z", {
					"conditional_intensity:buy": 0.18,
				}),
			],
			"hawkes",
			"BTC/USD",
		);

		expect(Object.fromEntries(merged)).toEqual({
			"conditional_intensity:buy": 0.18,
		});
	});

	it("merges legacy flat rows when metrics is undefined", () => {
		const merged = mergeInspectorMetrics(
			[
				{
					source: "pumpdump",
					symbol: "BTC/USD",
					metric: "peak_score",
					at: "2026-07-25T01:00:00.000Z",
					raw: 0.87,
					normalized: null,
					uncertainty: null,
					validity: { state: "valid", readiness: "observation" },
					scale: { kind: "observation_window", from: "", through: "" },
				},
			],
			"pumpdump",
			"BTC/USD",
		);

		expect(Object.fromEntries(merged)).toEqual({
			peak_score: 0.87,
		});
	});
});

describe("paintInspectorMeters", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("removes meters absent from the current complete DRAW", () => {
		vi.stubGlobal("document", {
			createElement: () => fakeNode(),
		});

		const host = fakeNode();
		const meters = new Map<string, MeterParts>();

		paintInspectorMeters(
			host as unknown as HTMLElement,
			meters,
			new Map([
				["touch_quantity:buy", 0.6],
				["fill_volume:buy", 0.2],
			]),
		);

		const fill = meters.get("fill_volume:buy");
		expect(fill).toBeDefined();

		paintInspectorMeters(
			host as unknown as HTMLElement,
			meters,
			new Map([["touch_quantity:buy", 0.55]]),
		);

		expect(meters.has("fill_volume:buy")).toBe(false);
		expect(fill?.cell.removed).toBe(true);
		expect(meters.size).toBe(1);
	});
});
