import { describe, expect, it } from "vitest";
import { sectionHeaderVariants, sectionVariants } from "./section";

describe("sectionVariants", () => {
	it("owns its slice and hides its overflow by default", () => {
		const classes = sectionVariants();

		expect(classes).toContain("flex flex-col");
		expect(classes).toContain("min-h-0");
		expect(classes).toContain("overflow-hidden");
		expect(classes).toContain("bg-(--surface)");
	});

	it("leaves the scroll to an ancestor when it grows to its content", () => {
		expect(sectionVariants({ fit: "content" })).not.toContain(
			"overflow-hidden",
		);
	});
});

describe("sectionHeaderVariants", () => {
	it("draws its own rule by default", () => {
		expect(sectionHeaderVariants()).toContain("border-b");
		expect(sectionHeaderVariants()).toContain("px-3 py-2");
	});

	/*
	The rule is the header's, not the body's. A header that could not turn it off
	is why a padded column ended up with a stray line across it.
	*/
	it("can drop the rule for a header inside padded content", () => {
		expect(sectionHeaderVariants({ rule: false })).not.toContain("border-b");
	});

	it("stays put over a scrolling column when sticky", () => {
		const classes = sectionHeaderVariants({ sticky: true });

		expect(classes).toContain("sticky");
		expect(classes).toContain("top-0");
		expect(classes).toContain("bg-(--surface)");
	});

	it("supports a bare header that supplies no box of its own", () => {
		expect(sectionHeaderVariants({ size: "bare", rule: false })).not.toContain(
			"px-3",
		);
	});
});
