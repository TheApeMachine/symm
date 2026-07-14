import { describe, expect, it } from "vitest";
import { statVariants } from "./stat";

describe("statVariants", () => {
	it("applies tile surface classes", () => {
		expect(statVariants({ layout: "tile" })).toContain("border-(--line)");
		expect(statVariants({ layout: "tile" })).toContain("bg-(--surface)");
	});

	it("keeps metric layout unboxed", () => {
		expect(statVariants({ layout: "metric" })).toBe("");
	});
});
