import { describe, expect, it } from "vitest";
import { panelVariants } from "./panel";

describe("panelVariants", () => {
	it("composes sunken surface classes at the default size", () => {
		const classes = panelVariants();

		expect(classes).toContain("bg-(--sunken)");
		expect(classes).toContain("rounded-[4px]");
		expect(classes).toContain("p-3");
	});

	it("supports compact and roomy padding sizes", () => {
		expect(panelVariants({ size: "s" })).toContain("px-2");
		expect(panelVariants({ size: "lg" })).toContain("p-[13px]");
	});
});
