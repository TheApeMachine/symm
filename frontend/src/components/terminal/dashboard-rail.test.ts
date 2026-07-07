import { describe, expect, it } from "vitest";
import type { Position } from "#/collections/positions";
import type { Stop } from "#/collections/stops";
import { positionGaugeGeometry } from "./dashboard-rail";

const position = (mark: number, returnPct: number): Position => ({
	symbol: "XLM/USD",
	qty: 10,
	entry_price: 100,
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
		expect(geometry?.fillLo).toBe(50);
		expect(geometry?.fillHi).toBeCloseTo(72.5);
		expect(geometry?.aboveStop).toBe(true);
		expect(geometry?.rawMarkPrice).toBe(101.9);
	});

	it("ignores a zero mark and uses return_pct for geometry only", () => {
		const geometry = positionGaugeGeometry(position(0, 0.015), stop());

		expect(geometry).not.toBeNull();
		expect(geometry?.entryPct).toBe(25);
		expect(geometry?.markPct).toBeCloseTo(62.5);
		expect(geometry?.stopPct).toBe(50);
		expect(geometry?.fillLo).toBe(50);
		expect(geometry?.fillHi).toBeCloseTo(62.5);
		expect(geometry?.aboveStop).toBe(true);
		expect(geometry?.rawMarkPrice).toBeNull();
	});
});
