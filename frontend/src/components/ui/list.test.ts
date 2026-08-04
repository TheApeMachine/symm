import { describe, expect, it } from "vitest";
import { listItemVariants, listOptionVariants } from "./list";

describe("listItemVariants", () => {
	it("takes hover only while it is interactive", () => {
		expect(listItemVariants()).toContain("hover:bg-(--sunken)");
		expect(listItemVariants({ interactive: false })).not.toContain("hover:");
	});
});

describe("listOptionVariants", () => {
	it("reserves a transparent border so selection does not shift the row", () => {
		expect(listOptionVariants()).toContain("border border-transparent");
	});

	it("marks the keyboard cursor with the accent", () => {
		const classes = listOptionVariants({ selected: true });

		expect(classes).toContain("border-(--acc)");
		expect(classes).toContain(
			"bg-[color-mix(in_srgb,var(--acc)_10%,transparent)]",
		);
	});

	it("offers a compact row for dropdowns", () => {
		expect(listOptionVariants({ size: "s" })).toContain("px-2.5 py-1.5");
		expect(listOptionVariants({ size: "m" })).toContain("px-3 py-2.5");
	});
});
