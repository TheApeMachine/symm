import { describe, expect, it } from "vitest";
import { modalPanelVariants, modalScrimVariants } from "./modal";
import { overlayContentVariants, overlayVariants } from "./overlay";

describe("overlayVariants", () => {
	it("dims the surface behind it by default", () => {
		const classes = overlayVariants();

		expect(classes).toContain("absolute");
		expect(classes).toContain("inset-0");
		expect(classes).toContain("backdrop-blur-[3px]");
		expect(classes).toContain(
			"bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)]",
		);
	});

	it("offers a heavier scrim for a palette that must own the screen", () => {
		expect(overlayVariants({ variant: "heavy" })).toContain(
			"bg-[color-mix(in_srgb,var(--sunken)_64%,transparent)]",
		);
	});
});

describe("overlayContentVariants", () => {
	/*
	The wrapper must stay transparent to the pointer: a narrow panel leaves scrim
	either side of it, and clicking there has to dismiss.
	*/
	it("passes pointer events through to the scrim", () => {
		expect(overlayContentVariants()).toContain("pointer-events-none");
	});

	it("drops a palette under the eye line rather than centring it", () => {
		expect(overlayContentVariants({ align: "top" })).toContain("items-start");
		expect(overlayContentVariants({ align: "center" })).toContain(
			"items-center",
		);
	});
});

describe("modal built on overlay", () => {
	it("keeps the scrim exported under its old name", () => {
		expect(modalScrimVariants()).toBe(overlayVariants());
	});

	it("re-enables pointer events on the panel itself", () => {
		expect(modalPanelVariants()).toContain("pointer-events-auto");
	});
});
