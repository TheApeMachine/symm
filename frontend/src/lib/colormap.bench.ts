import { bench, describe } from "vitest";
import { colormap, colormapCss } from "./colormap";

describe("colormap", () => {
	const values = Array.from({ length: 24 }, (_, index) => index / 23);

	bench("maps 24 cross-section strengths to rgb triples", () => {
		for (const value of values) {
			colormap(value);
		}
	});

	bench("maps 24 cross-section strengths to css rgb strings", () => {
		for (const value of values) {
			colormapCss(value);
		}
	});
});
