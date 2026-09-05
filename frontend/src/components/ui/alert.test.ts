import { describe, expect, it } from "vitest";
import { alertVariants } from "./alert";

describe("alertVariants", () => {
	/*
	The band is tinted from the same recipe as Badge. If these drift apart, a
	failure starts looking like a different kind of event depending on where it
	is reported, which is the thing this component exists to stop.
	*/
	it("tints the plate from the tone at badge strengths", () => {
		const classes = alertVariants();

		expect(classes).toContain("[--alert-tone:var(--error)]");
		expect(classes).toContain(
			"border-[color:color-mix(in_srgb,var(--alert-tone)_40%,transparent)]",
		);
		expect(classes).toContain(
			"bg-[color:color-mix(in_srgb,var(--alert-tone)_12%,transparent)]",
		);
		expect(classes).toContain("text-[color:var(--alert-tone)]");
	});

	it("defaults to error and retones on request", () => {
		expect(alertVariants({ variant: "warning" })).toContain(
			"[--alert-tone:var(--warning)]",
		);
		expect(alertVariants({ variant: "info" })).toContain(
			"[--alert-tone:var(--info)]",
		);
	});

	it("draws its own rule unless it is the last band in a container", () => {
		expect(alertVariants()).toContain("border-b");
		expect(alertVariants({ rule: false })).toContain("border-b-0");
	});
});

describe("Alert icon defaults", () => {
	/*
	The glyph marks a failure, not the component. If info and success bands grew
	one too, it would stop distinguishing anything.
	*/
	it("registers the failure glyph in the icon set", async () => {
		const { ICON_NAMES } = await import("./icon");

		expect(ICON_NAMES).toContain("broken");
	});
});
