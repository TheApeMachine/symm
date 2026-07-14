import { describe, expect, it } from "vitest";
import {
	colormap,
	colormapCss,
	DEFAULT_ACCENT_HEX,
	heatmapForeground,
	hexToRgb,
} from "./colormap";

describe("hexToRgb", () => {
	it("parses the default accent hex", () => {
		expect(hexToRgb(DEFAULT_ACCENT_HEX)).toEqual([232, 163, 61]);
	});
});

describe("colormap", () => {
	it("returns the sunken brown stop at zero", () => {
		expect(colormap(0)).toEqual([14, 12, 10]);
	});

	it("returns navy near the low-mid stop", () => {
		const [red, green, blue] = colormap(0.4);

		expect(red).toBe(26);
		expect(green).toBe(34);
		expect(blue).toBe(50);
	});

	it("returns teal near the mid stop", () => {
		const [red, green, blue] = colormap(0.6);

		expect(red).toBe(42);
		expect(green).toBe(106);
		expect(blue).toBe(129);
	});

	it("returns accent at the high stop", () => {
		expect(colormap(0.8)).toEqual(hexToRgb(DEFAULT_ACCENT_HEX));
	});

	it("clamps out-of-range values", () => {
		expect(colormap(-0.5)).toEqual(colormap(0));
		expect(colormap(1.5)).toEqual(colormap(1));
	});
});

describe("colormapCss", () => {
	it("renders integer rgb channels", () => {
		expect(colormapCss(0)).toBe("rgb(14,12,10)");
		expect(colormapCss(0.8)).toBe("rgb(232,163,61)");
	});
});

describe("heatmapForeground", () => {
	it("uses dark ink on bright cells and muted ink on cool cells", () => {
		expect(heatmapForeground(0.7)).toBe("#14110f");
		expect(heatmapForeground(0.4)).toBe("var(--f3)");
	});
});
