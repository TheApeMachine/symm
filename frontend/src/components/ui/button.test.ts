import { describe, expect, it } from "vitest";
import { buttonVariants } from "./button";

describe("buttonVariants", () => {
	it("keeps chrome and meaning on separate axes", () => {
		const classes = buttonVariants({ variant: "solid", tone: "error" });

		expect(classes).toContain("[--button-tone:var(--error)]");
		expect(classes).toContain(
			"bg-[color:color-mix(in_srgb,var(--button-tone)_16%,transparent)]",
		);
	});

	it("reads the same tone property from every variant", () => {
		for (const variant of ["solid", "outline", "quiet", "bare"] as const) {
			expect(buttonVariants({ variant, tone: "accent" })).toContain(
				"[--button-tone:var(--acc)]",
			);
		}
	});

	it("strips padding from a bare control whatever size it asked for", () => {
		const classes = buttonVariants({ variant: "bare", size: "lg" });

		expect(classes).toContain("p-0");
		expect(classes.lastIndexOf("p-0")).toBeGreaterThan(
			classes.indexOf("px-3 py-1.5"),
		);
	});

	it("sizes an icon control as a box rather than as a line of text", () => {
		expect(buttonVariants({ shape: "icon", size: "s" })).toContain(
			"size-[25px]",
		);
		expect(buttonVariants({ shape: "icon", size: "lg" })).toContain("size-8");
	});

	it("defaults to a quiet control at size s", () => {
		const classes = buttonVariants();

		expect(classes).toContain("border-transparent");
		expect(classes).toContain("hover:bg-(--raised)");
		expect(classes).toContain("text-[11px]");
	});
});
