import { describe, expect, it } from "vitest";
import { ratioBarTrackVariants } from "./ratio-bar";

describe("ratioBarTrackVariants", () => {
	it("applies size classes to the track", () => {
		expect(ratioBarTrackVariants({ size: "xs" })).toContain("h-1");
		expect(ratioBarTrackVariants({ size: "s" })).toContain("h-1.5");
		expect(ratioBarTrackVariants({ size: "m" })).toContain("h-2");
		expect(ratioBarTrackVariants({ size: "lg" })).toContain("h-3");
	});

	it("uses default s size and rounded-full line styling", () => {
		const classes = ratioBarTrackVariants();
		expect(classes).toContain("h-1.5");
		expect(classes).toContain("rounded-full");
		expect(classes).toContain("bg-(--line)");
	});
});
