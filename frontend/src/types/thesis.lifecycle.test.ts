import { describe, expect, it } from "vitest";
import { LIFECYCLE_MANAGING } from "#/types/thesis";

describe("LIFECYCLE_MANAGING", () => {
	it("covers position-management stages through closed", () => {
		expect(LIFECYCLE_MANAGING.has("managing")).toBe(true);
		expect(LIFECYCLE_MANAGING.has("exit_selected")).toBe(true);
		expect(LIFECYCLE_MANAGING.has("exit_submitted")).toBe(true);
		expect(LIFECYCLE_MANAGING.has("partially_exited")).toBe(true);
		expect(LIFECYCLE_MANAGING.has("closed")).toBe(true);
	});

	it("excludes entry and post-exit review stages", () => {
		expect(LIFECYCLE_MANAGING.has("entered")).toBe(false);
		expect(LIFECYCLE_MANAGING.has("post_exit_observation")).toBe(false);
		expect(LIFECYCLE_MANAGING.has("evaluated")).toBe(false);
	});
});
