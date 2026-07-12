import { describe, expect, it } from "vitest";
import { normalizeStops } from "./stops";

const stop = {
	symbol: "XLM/USD",
	stop_price: "0.24",
	peak_return: "0.10",
	stop_return: "0.04",
	momentum: "0.8",
	peak_momentum: "1.2",
	momentum_floor: "0.4",
	momentum_health: "0.5",
	momentum_active: true,
	peak_touch_count: 2,
	stagnation_max_touches: 4,
	stagnation_health: "0.5",
	stagnation_pending: false,
	stagnation_active: true,
};

describe("normalizeStops", () => {
	it("consumes symbol-keyed stop snapshots", () => {
		expect(normalizeStops({ "XLM/USD": stop })).toEqual({
			"XLM/USD": {
				...stop,
				stop_price: 0.24,
				peak_return: 0.1,
				stop_return: 0.04,
				momentum: 0.8,
				peak_momentum: 1.2,
				momentum_floor: 0.4,
				momentum_health: 0.5,
				stagnation_health: 0.5,
			},
		});
	});

	it("consumes array stop snapshots", () => {
		expect(normalizeStops([stop])["XLM/USD"]?.stop_price).toBe(0.24);
	});

	it("rejects missing stop financial fields", () => {
		expect(() => normalizeStops([{ ...stop, stop_price: undefined }])).toThrow(
			"stops[0].stop_price",
		);
	});
});
