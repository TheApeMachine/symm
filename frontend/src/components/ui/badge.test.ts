import { describe, expect, it } from "vitest";
import { badgeVariants } from "./badge";

describe("badgeVariants", () => {
	it("composes tailwind utility classes for variant and size", () => {
		const classes = badgeVariants({ variant: "success", size: "m" });

		expect(classes).toContain("[--badge-tone:var(--success)]");
		expect(classes).toContain("text-[11px]");
		expect(classes).toContain("uppercase");
	});

	it("defaults to info at size s", () => {
		const classes = badgeVariants();

		expect(classes).toContain("[--badge-tone:var(--info)]");
		expect(classes).toContain("text-[10px]");
	});

	it("composes muted disabled styling for inactive states", () => {
		const classes = badgeVariants({ variant: "disabled", size: "xs" });

		expect(classes).toContain("[--badge-tone:var(--f3)]");
		expect(classes).toContain("border-(--line2)");
		expect(classes).toContain("bg-(--line)");
		expect(classes).toContain("text-(--f3)");
	});
});
