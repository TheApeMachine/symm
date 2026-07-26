import { describe, expect, it } from "vitest";
import type { Holding, Stop } from "#/collections/types";
import {
	createPositionGaugeElement,
	paintPositionHoldings,
	positionGaugeGeometry,
} from "./position-gauge";

const position = (mark: number, returnPct: number): Holding => ({
	symbol: "XLM/USD",
	qty: 10,
	entry_price: 100,
	entry_fee: 2.6,
	exit_fee: 2.6,
	mark,
	pnl: returnPct * 100,
	return_pct: returnPct,
});

const stop = (): Stop => ({
	symbol: "XLM/USD",
	stop_price: 101,
	peak_return: 0.02,
	stop_return: 0.01,
	momentum: 1,
	peak_momentum: 1,
	momentum_floor: 0.6,
	momentum_health: 1,
	momentum_active: true,
	peak_touch_count: 0,
	stagnation_max_touches: 3,
	stagnation_health: 1,
	stagnation_pending: false,
	stagnation_active: false,
});

describe("positionGaugeGeometry", () => {
	it("anchors the price gauge to entry, stop, peak, and mark returns", () => {
		const geometry = positionGaugeGeometry(position(101.9, 0.019), stop());

		expect(geometry).not.toBeNull();
		expect(geometry?.entryPct).toBe(25);
		expect(geometry?.markPct).toBeCloseTo(72.5);
		expect(geometry?.stopPct).toBe(50);
		expect(geometry?.peakPct).toBe(75);
		expect(geometry?.rawMarkPrice).toBe(101.9);
	});

	it("ignores a zero mark and uses return_pct for geometry only", () => {
		const geometry = positionGaugeGeometry(position(0, 0.015), stop());

		expect(geometry).not.toBeNull();
		expect(geometry?.entryPct).toBe(25);
		expect(geometry?.markPct).toBeCloseTo(62.5);
		expect(geometry?.stopPct).toBe(50);
		expect(geometry?.peakPct).toBe(75);
		expect(geometry?.rawMarkPrice).toBeNull();
	});

	it("scales mark distance from entry when no stop frame exists", () => {
		const geometry = positionGaugeGeometry(position(95, -0.05));

		expect(geometry).not.toBeNull();
		expect(geometry?.entryPct).toBeGreaterThan(geometry?.markPct ?? 0);
		expect(geometry?.stopPct).toBeNull();
		expect(geometry?.peakPct).toBeNull();
	});
});

describe("paintPositionHoldings", () => {
	it("paints restored map-shaped holdings with decimal strings", () => {
		document.body.replaceChildren(createPositionGaugeElement("EUL/USD", "USD"));

		paintPositionHoldings(
			{
				"EUL/USD": {
					symbol: "EUL/USD",
					qty: "9.85795428",
					entry_price: "1.729",
					entry_fee: "0.044315447670311994",
					exit_fee: "0",
					mark: "1.73",
					pnl: "0.00985795428",
					return_pct: "0.0005783337200618928",
					status: "open",
				},
			},
			"BTC/USD",
		);

		expect(document.body.textContent).toContain("P/L 0.0099 USD");
		expect(document.body.textContent).toContain("entry 1.729 / mark 1.7300");
	});
});
