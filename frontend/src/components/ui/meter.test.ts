import { describe, expect, it } from "vitest";
import { meterTrackVariants, meterVariants } from "./meter";

describe("meterTrackVariants", () => {
	it("applies semantic tone and size classes to the track", () => {
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"[--meter-tone:var(--success)]",
		);
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"h-1.5",
		);
	});
});

describe("meterVariants", () => {
	it("applies layout classes", () => {
		expect(meterVariants({ layout: "inline" })).toBe("flex items-center gap-2");
	});
});
