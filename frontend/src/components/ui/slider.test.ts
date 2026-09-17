import { describe, expect, it } from "vitest";
import { sliderFieldVariants, sliderVariants } from "./slider";

describe("sliderVariants", () => {
	it("applies tone and size classes to the range slider", () => {
		const classes = sliderVariants({ tone: "brand", size: "m" });
		expect(classes).toContain("accent-(--brand)");
		expect(classes).toContain("h-5");
	});

	it("uses default acc tone and s size", () => {
		const classes = sliderVariants();
		expect(classes).toContain("accent-(--acc)");
		expect(classes).toContain("h-4");
	});
});

describe("sliderFieldVariants", () => {
	it("applies box variant styling", () => {
		const classes = sliderFieldVariants({ variant: "box" });
		expect(classes).toContain("border-(--line)");
		expect(classes).toContain("bg-(--surface)");
	});
});
