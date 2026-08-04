import { describe, expect, it } from "vitest";
import { badgeVariants } from "./badge";
import { chipVariants } from "./chip";

describe("chipVariants", () => {
	it("stays neutral, in mono", () => {
		const classes = chipVariants();

		expect(classes).toContain("font-mono");
		expect(classes).toContain("text-(--f3)");
		expect(classes).toContain("border-(--line)");
	});

	/*
	A Chip must not be reachable as a status pill. The moment it grows a tone
	scale, header readouts start competing with real state for the same colours.
	*/
	it("carries no semantic tone of its own", () => {
		expect(chipVariants()).not.toMatch(/--(chip|badge)-tone/);
		expect(badgeVariants()).toContain("--badge-tone");
	});

	it("drops its box when bare", () => {
		const classes = chipVariants({ variant: "bare", size: "lg" });

		expect(classes).not.toContain("border-(--line)");
		expect(classes.lastIndexOf("p-0")).toBeGreaterThan(
			classes.indexOf("px-2.5"),
		);
	});
});
