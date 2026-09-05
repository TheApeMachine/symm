import { describe, expect, it } from "vitest";
import { SURFACE_ITEMS } from "./terminal-nav";

describe("SURFACE_ITEMS", () => {
	it("keeps the dashboard and forward learning on separate routes", () => {
		expect(SURFACE_ITEMS.find((item) => item.key === "dashboard")?.to).toBe(
			"/",
		);
		expect(SURFACE_ITEMS.find((item) => item.key === "learning")?.to).toBe(
			"/learning",
		);
	});

	it("does not advertise retired regulator or market graph surfaces", () => {
		const labels = SURFACE_ITEMS.map((item) => item.label);

		expect(labels).not.toContain("Global regulator");
		expect(labels).not.toContain("Market graph");
	});
});
