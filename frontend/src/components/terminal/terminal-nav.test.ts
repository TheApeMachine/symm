import { describe, expect, it } from "vitest";
import { SURFACE_ITEMS } from "./terminal-nav";

describe("SURFACE_ITEMS", () => {
	it("does not advertise retired regulator or market graph surfaces", () => {
		const labels = SURFACE_ITEMS.map((item) => item.label);

		expect(labels).not.toContain("Global regulator");
		expect(labels).not.toContain("Market graph");
	});
});
