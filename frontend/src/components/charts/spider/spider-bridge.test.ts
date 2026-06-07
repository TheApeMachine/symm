import { describe, expect, it } from "vitest";

import {
	attachSpiderBridge,
	ingestRegimeWire,
	REGIME_MARKET_SYMBOL,
	scaleSpiderRadarValues,
} from "#/components/charts/spider/spider-bridge";

const axes = [
	"volatility",
	"trend",
	"bullish",
	"bearish",
	"choppiness",
] as const;

describe("ingestRegimeWire", () => {
	it("accepts the market regime frame", () => {
		const bridge = {
			setAll: () => {},
			ready: false,
			pending: null,
			latest: {},
		};

		ingestRegimeWire(
			bridge,
			{
				chart: "regime",
				symbol: REGIME_MARKET_SYMBOL,
				volatility: 0.2,
				trend: 0.1,
				bullish: 0.3,
				bearish: 0.4,
				choppiness: 0.5,
			},
			axes,
		);

		expect(bridge.pending).toEqual({
			volatility: 0.2,
			trend: 0.1,
			bullish: 0.3,
			bearish: 0.4,
			choppiness: 0.5,
		});
	});

	it("rejects per-symbol regime frames", () => {
		const bridge = {
			setAll: () => {},
			ready: false,
			pending: null,
			latest: {},
		};

		ingestRegimeWire(
			bridge,
			{
				chart: "regime",
				symbol: "BTC/EUR",
				volatility: 0.9,
				trend: 0.5,
				bullish: 0.1,
				bearish: 0.1,
				choppiness: 0.1,
			},
			axes,
		);

		expect(bridge.pending).toBeNull();
	});
});

describe("attachSpiderBridge", () => {
	it("replays pending values when the chart attaches", () => {
		const bridge = {
			setAll: () => {},
			ready: false,
			pending: null,
			latest: {},
		};
		const applied: Record<string, number>[] = [];

		ingestRegimeWire(bridge, { chart: "regime", volatility: 0.2, trend: 0.1 }, [
			"volatility",
			"trend",
		]);
		ingestRegimeWire(bridge, { chart: "regime", volatility: 0.8, trend: 0.4 }, [
			"volatility",
			"trend",
		]);

		attachSpiderBridge(bridge, (values) => {
			applied.push(values);
		});

		expect(applied).toEqual([{ volatility: 0.8, trend: 0.4 }]);
		expect(bridge.pending).toBeNull();
	});
});

describe("scaleSpiderRadarValues", () => {
	it("maps axis values to radar radii", () => {
		expect(
			scaleSpiderRadarValues(["volatility", "trend"], {
				volatility: 0.2,
				trend: 0.1,
			}),
		).toEqual([20, 10]);
	});
});
