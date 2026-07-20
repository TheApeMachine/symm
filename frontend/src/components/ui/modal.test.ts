import { describe, expect, it } from "vitest";
import { modalPanelVariants, modalScrimVariants } from "./modal";

describe("modalScrimVariants", () => {
	it("applies dim overlay classes by default", () => {
		const classes = modalScrimVariants();

		expect(classes).toContain("absolute");
		expect(classes).toContain("inset-0");
		expect(classes).toContain("backdrop-blur-[3px]");
		expect(classes).toContain(
			"bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)]",
		);
	});

	it("supports a solid scrim variant", () => {
		expect(modalScrimVariants({ variant: "solid" })).toContain(
			"bg-(--sunken)",
		);
	});
});

describe("modalPanelVariants", () => {
	it("applies the default panel surface and medium width", () => {
		const classes = modalPanelVariants();

		expect(classes).toContain("bg-(--surface)");
		expect(classes).toContain("border-(--line2)");
		expect(classes).toContain("max-w-[452px]");
	});

	it("supports compact and roomy panel sizes", () => {
		expect(modalPanelVariants({ size: "s" })).toContain("max-w-[360px]");
		expect(modalPanelVariants({ size: "lg" })).toContain("max-w-[560px]");
	});
});
