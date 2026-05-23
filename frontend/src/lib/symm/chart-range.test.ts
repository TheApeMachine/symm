import { describe, expect, it } from "vitest";

import {
	candleYRange,
	PulseValueClip,
	pulseYRange,
} from "#/lib/symm/chart-range";

describe("candleYRange", () => {
	it("ignores entry stop far from live price band", () => {
		const range = candleYRange(0.0063, 0.0066, [0.00649, 0.0065]);

		expect(range).not.toBeNull();
		expect(range?.min).toBeGreaterThan(0.006);
		expect(range?.max).toBeLessThan(0.007);
	});

	it("expands around a single live price", () => {
		const range = candleYRange(Number.NaN, Number.NaN, [0.0664]);

		expect(range?.min).toBeLessThan(0.0664);
		expect(range?.max).toBeGreaterThan(0.0664);
	});
});

describe("PulseValueClip", () => {
	it("clips div outliers after warm history", () => {
		const clip = new PulseValueClip();

		for (let index = 0; index < 12; index++) {
			clip.clip(-50 - index);
		}

		const clipped = clip.clip(-2500);

		expect(clipped).toBeGreaterThan(-500);
		expect(clipped).toBeLessThan(0);
	});
});

describe("pulseYRange", () => {
	it("returns padded bounds for pulse series", () => {
		const range = pulseYRange([0.1, 0.2], [0.05, 0.15], [-40, -55]);

		expect(range?.min).toBeLessThan(-55);
		expect(range?.max).toBeGreaterThan(0.2);
	});
});
