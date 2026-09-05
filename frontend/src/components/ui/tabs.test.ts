import { describe, expect, it } from "vitest";
import { tabsVariants, tabVariants } from "./tabs";

describe("tabsVariants", () => {
	it("draws the track recessed behind a hairline", () => {
		const classes = tabsVariants();

		expect(classes).toContain("bg-(--sunken)");
		expect(classes).toContain("border-(--line)");
	});

	it("turns the rail for a vertical control", () => {
		expect(tabsVariants({ orientation: "vertical" })).toContain("flex-col");
	});
});

describe("tabVariants", () => {
	/*
	The selected tab has to be legible as a raised plate, not merely as a colour
	change: that depth is the whole reason this is a segmented control rather
	than a row of buttons.
	*/
	it("lifts the active tab onto the raised surface", () => {
		const classes = tabVariants({ active: true });

		expect(classes).toContain("bg-(--raised)");
		expect(classes).toContain("shadow-xs");
		expect(classes).toContain("font-semibold");
		expect(classes).toContain("text-(--tab-tone)");
	});

	it("leaves an inactive tab flat and quiet", () => {
		const classes = tabVariants({ active: false });

		expect(classes).not.toContain("bg-(--raised)");
		expect(classes).toContain("text-(--f4)");
	});

	it("defaults to the terminal accent and retones on request", () => {
		expect(tabVariants()).toContain("[--tab-tone:var(--acc)]");
		expect(tabVariants({ tone: "info" })).toContain("[--tab-tone:var(--info)]");
	});
});
