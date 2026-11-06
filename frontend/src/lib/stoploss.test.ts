import { describe, expect, it } from "vitest";
import { normalizeStoploss } from "./stoploss";

describe("normalizeStoploss", () => {
	it("maps Go PascalCase + iota State onto UI telemetry", () => {
		const stop = normalizeStoploss({
			State: 2,
			CurrentPnLPct: 1.2,
			PeakPnLPct: 2.0,
			FloorPnLPct: 1.0,
			InitialPnLPct: -0.3,
		});

		expect(stop?.state).toBe("TakeProfit");
		expect(stop?.current_pnl).toBe(1.2);
		expect(stop?.peak_pnl).toBe(2.0);
		expect(stop?.floor_pnl).toBe(1.0);
		expect(stop?.friction).toBe(0.3);
		expect(stop?.has_floor).toBe(true);
		expect(stop?.room_pct).toBeCloseTo(0.2, 5);
	});

	it("accepts an already-normalized DynamicStoploss object", () => {
		const stop = normalizeStoploss({
			state: "Armed",
			friction: 0.2,
			current_pnl: -0.1,
			peak_pnl: 0.4,
			floor_pnl: -0.5,
			has_floor: true,
			room_pct: 0.4,
			proximity: 0.8,
		});

		expect(stop?.state).toBe("Armed");
		expect(stop?.proximity).toBe(0.8);
	});

	it("Healthy has no soft floor yet", () => {
		const stop = normalizeStoploss({
			State: 0,
			CurrentPnLPct: -0.22,
			PeakPnLPct: -0.08,
			FloorPnLPct: -0.08,
			InitialPnLPct: -0.08,
		});

		expect(stop?.state).toBe("Healthy");
		expect(stop?.has_floor).toBe(false);
		expect(stop?.room_pct).toBeCloseTo(-0.14, 5);
	});

	it("Armed and Triggered codes map exactly", () => {
		expect(normalizeStoploss({
			State: 1,
			CurrentPnLPct: 0.8,
			PeakPnLPct: 0.9,
			FloorPnLPct: 0.45,
			InitialPnLPct: -0.2,
		})?.state).toBe("Armed");

		expect(normalizeStoploss({
			State: 3,
			CurrentPnLPct: 0.3,
			PeakPnLPct: 1.2,
			FloorPnLPct: 0.6,
			InitialPnLPct: -0.2,
		})?.state).toBe("Triggered");
	});
});
