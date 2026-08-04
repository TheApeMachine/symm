import { describe, expect, it } from "vitest";
import { DOT_SIZE_FOR, dotVariants } from "./dot";
import type { Size } from "./types";

const SIZES: Size[] = ["xxs", "xs", "s", "m", "lg", "xl", "xxl"];

describe("dotVariants", () => {
	it("routes every tone through one custom property", () => {
		expect(dotVariants({ variant: "success" })).toContain(
			"[--dot-tone:var(--success)]",
		);
		expect(dotVariants({ variant: "error" })).toContain(
			"[--dot-tone:var(--error)]",
		);
		expect(dotVariants({ variant: "info" })).toContain(
			"[--dot-tone:var(--info)]",
		);
	});

	/*
	These four classes are what kernel-list's data-paint-class toggles at runtime.
	They only exist in the stylesheet because they are literals in dot.tsx, so a
	rename here silently kills the painted validity dot — hence the test.
	*/
	it("emits the tone classes the paint layer toggles", () => {
		for (const variant of ["success", "error", "info", "muted"] as const) {
			expect(dotVariants({ variant })).toMatch(/\[--dot-tone:var\(--[\w-]+\)\]/);
		}
	});

	it("fills from the tone property rather than a literal colour", () => {
		expect(dotVariants({ fill: "solid" })).toContain("bg-(--dot-tone)");
		expect(dotVariants({ fill: "hollow" })).toContain("border-(--dot-tone)");
	});

	it("defines a size for every step of the scale", () => {
		for (const size of SIZES) {
			expect(dotVariants({ size })).toMatch(/size-[\d.]+/);
		}
	});
});

describe("DOT_SIZE_FOR", () => {
	it("never returns a dot larger than its container", () => {
		for (const [index, size] of SIZES.entries()) {
			expect(SIZES.indexOf(DOT_SIZE_FOR[size])).toBeLessThanOrEqual(index);
		}
	});
});
