import { describe, expect, it } from "vitest";

import {
	attachSpiderBridge,
	createSpiderBridge,
	deliverRegimeWire,
	parseRegimeWire,
	REGIME_CHART_SYMBOL,
	scaleSpiderRadarValues,
} from "#/components/charts/spider/spider-bridge";

const REGIME_SOURCES = [
	"volatility",
	"trend",
	"bullish",
	"bearish",
	"choppiness",
] as const;

describe("parseRegimeWire", () => {
	it("accepts regime frames for the anchor symbol only", () => {
		expect(
			parseRegimeWire(
				{
					chart: "regime",
					symbol: REGIME_CHART_SYMBOL,
					volatility: 0.5,
					trend: 0.25,
					bullish: 0.25,
					bearish: 0,
					choppiness: 1,
				},
				REGIME_SOURCES,
			),
		).toEqual({
			volatility: 0.5,
			trend: 0.25,
			bullish: 0.25,
			bearish: 0,
			choppiness: 1,
		});
	});

	it("rejects regime frames for non-anchor symbols", () => {
		expect(
			parseRegimeWire(
				{
					chart: "regime",
					symbol: "ETH/EUR",
					volatility: 0.9,
					trend: 0.9,
					bullish: 0.9,
					bearish: 0.9,
					choppiness: 0.9,
				},
				REGIME_SOURCES,
			),
		).toBeNull();
	});
});

describe("scaleSpiderRadarValues", () => {
	it("maps regime axes to 0-100 radial values in source order", () => {
		expect(
			scaleSpiderRadarValues(REGIME_SOURCES, {
				volatility: 0.5,
				trend: 0.25,
				bullish: 0.25,
				bearish: 0,
				choppiness: 1,
			}),
		).toEqual([50, 25, 25, 0, 100]);
	});
});

describe("deliverRegimeWire", () => {
	it("buffers the latest anchor snapshot until the chart is ready", () => {
		const bridge = createSpiderBridge();
		const applied: Record<string, number>[] = [];

		deliverRegimeWire(bridge, { volatility: 0.2, trend: 0.1 });
		deliverRegimeWire(bridge, { volatility: 0.8, trend: 0.4 });

		expect(bridge.pending).toEqual({ volatility: 0.8, trend: 0.4 });
		expect(bridge.latest).toEqual({ volatility: 0.8, trend: 0.4 });

		attachSpiderBridge(bridge, (values) => {
			applied.push(values);
		});

		expect(applied).toEqual([{ volatility: 0.8, trend: 0.4 }]);
		expect(bridge.pending).toBeNull();
	});

	it("keeps the latest snapshot across chart detach", () => {
		const bridge = createSpiderBridge();
		const applied: Record<string, number>[] = [];

		attachSpiderBridge(bridge, (values) => {
			applied.push(values);
		});
		bridge.setAll({ volatility: 0.6, trend: 0.3 });

		bridge.setAll = () => {};
		bridge.ready = false;

		deliverRegimeWire(bridge, { volatility: 0.9, trend: 0.5 });

		attachSpiderBridge(bridge, (values) => {
			applied.push(values);
		});

		expect(applied.at(-1)).toEqual({ volatility: 0.9, trend: 0.5 });
	});
});
