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
});
