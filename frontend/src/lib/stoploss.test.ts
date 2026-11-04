import { describe, expect, it } from "vitest";
import { normalizeStoploss } from "./stoploss";

describe("normalizeStoploss", () => {
	it("maps Go PascalCase + iota State onto UI telemetry", () => {
		const stop = normalizeStoploss({
			State: 4,
			CurrentPnLPct: 1.2,
			PeakPnLPct: 2.0,
			FloorPnLPct: 0.5,
			InitialPnLPct: -0.3,
			MaxLossPct: -2,
			ProfitTargetPct: 2,
		});

		expect(stop?.state).toBe("Protected");
		expect(stop?.current_pnl).toBe(1.2);
		expect(stop?.peak_pnl).toBe(2.0);
		expect(stop?.floor_pnl).toBe(0.5);
		expect(stop?.friction).toBe(0.3);
		expect(stop?.has_floor).toBe(true);
		expect(stop?.room_pct).toBeCloseTo(0.7, 5);
	});

	it("accepts an already-normalized DynamicStoploss object", () => {
		const stop = normalizeStoploss({
			state: "Dangerous",
			friction: 0.2,
			current_pnl: -0.1,
			peak_pnl: 0.4,
			floor_pnl: -0.5,
			has_floor: true,
			room_pct: 0.4,
			proximity: 0.8,
			downside_pressure: 0,
			downside_magnitude_ratio: 0,
			consecutive_down: 0,
		});

		expect(stop?.state).toBe("Dangerous");
		expect(stop?.proximity).toBe(0.8);
	});

	it("does not treat the hard rail as an armed soft floor", () => {
		const stop = normalizeStoploss({
			State: 1,
			CurrentPnLPct: -0.22,
			PeakPnLPct: -0.08,
			FloorPnLPct: -2.5,
			InitialPnLPct: -0.08,
			MaxLossPct: -2.5,
			ProfitTargetPct: 10,
		});

		expect(stop?.state).toBe("Healthy");
		expect(stop?.has_floor).toBe(false);
		expect(stop?.room_pct).toBeCloseTo(2.28, 5);
	});
});
