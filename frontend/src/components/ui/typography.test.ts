import { describe, expect, it } from "vitest";
import { labelVariants, monoVariants } from "./typography";
import type { Size } from "./types";

const SIZES: Size[] = ["xxs", "xs", "s", "m", "lg", "xl", "xxl"];

describe("labelVariants", () => {
	it("is always upper case and tracked out", () => {
		for (const size of SIZES) {
			const classes = labelVariants({ size });

			expect(classes).toContain("uppercase");
			expect(classes).toMatch(/tracking-\[[\d.]+em\]/);
		}
	});

	it("loosens tracking as the type grows", () => {
		const tracking = (size: Size) =>
			Number(/tracking-\[([\d.]+)em\]/.exec(labelVariants({ size }))?.[1]);

		expect(tracking("xxl")).toBeLessThan(tracking("s"));
	});

	it("defaults to the quiet overline the rails use", () => {
		const classes = labelVariants();

		expect(classes).toContain("text-(--f3)");
		expect(classes).toContain("font-semibold");
		expect(classes).toContain("text-[10px]");
	});
});

describe("monoVariants", () => {
	/*
	A readout is written several times a second. Proportional figures would
	reflow its neighbours on every digit change.
	*/
	it("uses tabular figures at every size", () => {
		for (const size of SIZES) {
			expect(monoVariants({ size })).toContain("tabular-nums");
			expect(monoVariants({ size })).toContain("font-mono");
		}
	});

	it("carries the directional tones a P&L readout needs", () => {
		expect(monoVariants({ tone: "up" })).toContain("text-(--up)");
		expect(monoVariants({ tone: "down" })).toContain("text-(--down)");
	});
});
