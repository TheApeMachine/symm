import { describe, expect, it } from "vitest";
import { navItemVariants, navVariants } from "./nav";
import { toolbarVariants } from "./toolbar";

describe("navVariants", () => {
	it("is a fixed-width rail on a surface", () => {
		const classes = navVariants();

		expect(classes).toContain("shrink-0");
		expect(classes).toContain("flex-col");
		expect(classes).toContain("bg-(--surface)");
		expect(classes).toContain("border-r");
	});
});

describe("navItemVariants", () => {
	/*
	Both states reserve a border. If the inactive one did not, selecting an entry
	would push the whole rail a pixel wider.
	*/
	it("reserves the same box in both states", () => {
		expect(navItemVariants({ active: true })).toContain("border-(--line2)");
		expect(navItemVariants({ active: false })).toContain("border-transparent");
	});

	it("lifts the active entry onto the raised surface", () => {
		expect(navItemVariants({ active: true })).toContain("bg-(--raised)");
		expect(navItemVariants({ active: true })).toContain("text-(--f1)");
		expect(navItemVariants({ active: false })).toContain("text-(--f3)");
	});
});

describe("toolbarVariants", () => {
	it("scrolls rather than wraps", () => {
		expect(toolbarVariants()).toContain("overflow-x-auto");
		expect(toolbarVariants()).not.toContain("flex-wrap");
	});

	it("keeps a rule under every size", () => {
		for (const size of ["s", "m", "lg"] as const) {
			expect(toolbarVariants({ size })).toContain("border-b");
			expect(toolbarVariants({ size })).toContain("shrink-0");
		}
	});
});
